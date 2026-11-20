// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// client.go — Envoltura sobre whatsmeow: sesión SQLite, emparejamiento
// (QR y código de 8 dígitos), reconexión y envíos con fallback.
//
// Equivalencias con el Shin-MD de JS:
//
//	src/core/socket.js   →  este archivo (whatsmeow ya reconecta solo)
//	src/core/auth.js     →  store/sqlstore (SQLite con migraciones propias)
//	scripts/patch-baileys-pairing.cjs → NO HACE FALTA (no hay forks ni parches)
package wa

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"shin-go/internal/config"
	"shin-go/internal/consola"
	"shin-go/internal/xlog"
)

// PairDisplayName es cómo se presenta el bot en «Dispositivos vinculados».
//
// OJO: WhatsApp valida este campo. whatsmeow lo documenta en PairPhone
// («must be formatted as `Browser (OS)`, and only common browsers/OSes are
// allowed, the server will validate it and return 400 if it's wrong») y
// antes aquí ponía «Shin-MD (Go)», que NO cumple el formato: WhatsApp
// contestaba «info query returned status 400: bad-request» y nunca emitía
// el código de 8 letras. «Chrome (Linux)» es el valor que mautrix-whatsapp
// usa en producción; el SO es solo una etiqueta para la validación.
const PairDisplayName = "Chrome (Linux)"

type Client struct {
	waClient  *whatsmeow.Client
	Queue     *Queue
	IDs       *Resolver // traduce teléfono ↔ LID (ver identity.go)
	Log       *xlog.Logger
	StartedAt time.Time
	container *sqlstore.Container
	cfg       config.Config

	// baseCtx es el contexto del proceso: los handlers y los envíos lo heredan
	// para que un Ctrl+C los cancele a todos. Antes cada envío usaba
	// context.Background() y un cierre limpio no cancelaba nada.
	baseCtx context.Context

	onMessage   func(ctx context.Context, msg *IncomingMessage)
	onGroupInfo func(ctx context.Context, evt *GroupChange)

	// qrVisible: el usuario eligió vincular por QR en el menú. Solo entonces
	// se dibuja el código en la terminal; con código de 8 letras no se pinta
	// nada (sería ruido y un QR que nadie va a escanear).
	qrVisible atomic.Bool
}

// New abre la base de sesión y construye el cliente. No se conecta todavía.
//
// `mem` es la base del bot (economía, settings, identidades); se usa para que
// las parejas LID ↔ teléfono sobrevivan a la sesión. Puede ser nil.
func New(ctx context.Context, cfg config.Config, log *xlog.Logger, mem IdentityStore) (*Client, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.AuthFile), 0o700); err != nil {
		return nil, fmt.Errorf("crear carpeta de sesión: %w", err)
	}

	// modernc.org/sqlite es SQLite en Go PURO: sin cgo, sin compilar nada,
	// compila igual en Windows, en un VPS y en Termux (arm64).
	dsn := "file:" + cfg.AuthFile + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)"
	container, err := sqlstore.New(ctx, "sqlite", dsn, waLog.Stdout("wa-db", "ERROR", true))
	if err != nil {
		return nil, fmt.Errorf("abrir sesión SQLite: %w", err)
	}

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("leer dispositivo: %w", err)
	}

	// auth.db guarda las llaves de la sesión: si el archivo queda legible para
	// otros usuarios del servidor, cualquiera puede suplantar el número.
	secureSessionFiles(cfg.AuthFile, log)

	// El mapeo LID ↔ teléfono pasa por nuestro envoltorio: así todo lo que la
	// librería aprende se guarda también en la base del bot, y lo aprendido en
	// una sesión anterior se puede usar en la siguiente.
	device.LIDs = newLIDStore(device.LIDs, mem, log)

	cli := whatsmeow.NewClient(device, waLog.Stdout("whatsmeow", "WARN", true))
	cli.EnableAutoReconnect = true // clave: nada de borrar la sesión por cortes de red

	c := &Client{
		waClient:  cli,
		Queue:     NewQueue(cfg),
		IDs:       NewResolver(mem, log),
		Log:       log,
		StartedAt: time.Now(),
		container: container,
		cfg:       cfg,
		baseCtx:   ctx,
	}

	// Igual que en Shin-MD: nunca se borra una sesión registrada por un corte
	// de red. whatsmeow reconecta; aquí solo se avisa y se deja registro.
	cli.AddEventHandler(func(raw any) {
		switch evt := raw.(type) {
		case *events.QR:
			// Solo llegan si hay un canal de QR activo y no hay sesión. Se
			// dibuja únicamente cuando el usuario pidió el QR en el menú.
			if c.qrVisible.Load() && len(evt.Codes) > 0 {
				PanelQR(os.Stdout, evt.Codes[0], c.rutaQR())
			}
		case *events.Connected:
			log.Ok("Conectado a WhatsApp")
		case *events.Disconnected:
			log.Warn("Desconectado — reconexión automática en curso (sesión intacta)")
		case *events.ConnectFailure:
			log.Warn(fmt.Sprintf("Fallo de conexión: %s (%s)", evt.Reason, evt.Message))
		case *events.LoggedOut:
			log.Err("Sesión cerrada desde el teléfono (LoggedOut) — hace falta vincular otra vez")
		case *events.Message:
			if c.onMessage != nil {
				c.onMessage(c.baseCtx, NormalizeMessage(evt, c.cfg.Prefixes))
			}
		case *events.GroupInfo:
			// La conversión al modelo interno sucede aquí, antes del callback del
			// router y de los oyentes de negocio.
			if c.onGroupInfo != nil {
				c.onGroupInfo(c.baseCtx, normalizeGroupChange(evt))
			}
		}
	})

	return c, nil
}

// secureSessionFiles pone 600 en la base de sesión y en sus archivos WAL/SHM.
func secureSessionFiles(path string, log *xlog.Logger) {
	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		if err := os.Chmod(f, 0o600); err != nil {
			log.Warn("no se pudieron ajustar los permisos de " + f + ": " + err.Error())
		}
	}
}

func (c *Client) OnMessage(fn func(ctx context.Context, msg *IncomingMessage)) { c.onMessage = fn }

// OnGroupInfo registra el manejador interno de cambios de grupo. El evento
// externo ya fue normalizado por el adaptador.
func (c *Client) OnGroupInfo(fn func(ctx context.Context, evt *GroupChange)) { c.onGroupInfo = fn }

func (c *Client) LoggedIn() bool { return c.waClient != nil && c.waClient.Store.ID != nil }

// Connected informa el estado de conexión sin exponer el cliente externo.
func (c *Client) Connected() bool { return c != nil && c.waClient != nil && c.waClient.IsConnected() }

// AccountName devuelve el nombre de perfil o, como reserva, el usuario del
// JID propio. Es lo único que necesita mostrar el arranque del bot.
func (c *Client) AccountName() string {
	if c == nil || c.waClient == nil || c.waClient.Store == nil {
		return ""
	}
	if name := c.waClient.Store.PushName; name != "" {
		return name
	}
	return c.OwnID().User
}

// OwnLID es el LID del propio bot (vacío si WhatsApp no lo dio todavía).
// Lo usan los comandos que necesitan saber cómo aparece el bot en un grupo.
func (c *Client) OwnLID() JID { return jidFromWhatsmeow(c.waClient.Store.GetLID().ToNonAD()) }

// OwnID es el JID del propio bot (vacío si aún no hay sesión). Sirve para no
// dejar que los comandos actúen sobre el bot: robarle, curarlo, etc.
func (c *Client) OwnID() JID {
	if c.waClient == nil || c.waClient.Store.ID == nil {
		return JID{}
	}
	return jidFromWhatsmeow(c.waClient.Store.ID.ToNonAD())
}

// Uptime: tiempo desde que arrancó el proceso (lo muestra .ping).
func (c *Client) Uptime() time.Duration { return time.Since(c.StartedAt) }

// Reconectar: cerrar el socket y volver a abrirlo. No toca la sesión guardada,
// así que es seguro pulsarlo desde la consola cuando la conexión se vea
// enredada (wifi que cambia, un portador que se queda dormido en el móvil).
func (c *Client) Reconectar(_ context.Context) error {
	if c.waClient == nil {
		return errors.New("sin cliente de WhatsApp que reconectar")
	}
	c.waClient.Disconnect()
	return c.waClient.Connect()
}

// Context devuelve el contexto del proceso, para lo que no viene de un mensaje.
func (c *Client) Context() context.Context { return c.baseCtx }

func (c *Client) Close() {
	c.waClient.Disconnect()
	if c.container != nil {
		_ = c.container.Close()
	}
}

// MetodoDeVinculacion: cómo se vinculó el bot en este arranque.
type MetodoDeVinculacion int

const (
	// VinculacionCodigo: código de 8 letras (el camino normal en Termux).
	VinculacionCodigo MetodoDeVinculacion = iota
	// VinculacionQR: se dibuja el QR y se escanea desde otro teléfono.
	VinculacionQR
)

// LoginPorQR: el otro camino para vincular sin escribir nada en el teléfono.
// Dibuja el QR en la terminal y espera a que se escanee; al terminar deja el
// cliente conectado, igual que Login.
//
// Aquí SÍ se usa el canal de QR de verdad (no solo como señal de «ya
// conectado»): cada código que emite whatsmeow se pinta. Cuando el canal
// manda «success», la sesión quedó guardada en SQLite y el bot sigue solo.
func (c *Client) LoginPorQR(ctx context.Context) error {
	if c.LoggedIn() {
		return c.waClient.Connect()
	}
	qr, err := c.waClient.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("preparar vinculación por QR: %w", err)
	}
	c.qrVisible.Store(true)
	defer c.qrVisible.Store(false)
	// El PNG es un plano de un solo uso: se borra al terminar (se vincule o no)
	// para que nadie escanee mañana un código caducado creyendo que vale.
	rutaQR := c.rutaQR()
	defer func() { _ = os.Remove(rutaQR) }()

	sp := consola.Spinner(os.Stdout, "abriendo la vinculación por QR…")
	if err := c.waClient.Connect(); err != nil {
		if sp != nil {
			sp.Fail("no hubo conexión")
		}
		return fmt.Errorf("conectar para vincular: %w", err)
	}
	if sp != nil {
		sp.Success("vinculación abierta")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case item, abierto := <-qr:
			if !abierto {
				return fmt.Errorf("la vinculación se cerró sin escanear el QR")
			}
			switch item.Event {
			case whatsmeow.QRChannelEventCode:
				// El evento QR (con el código) ya lo pinta el manejador de
				// eventos; aquí solo se evita el bucle cerrado.
				continue
			case whatsmeow.QRChannelSuccess.Event:
				consola.Bien("¡vinculado! el bot queda conectado")
				return nil
			case whatsmeow.QRChannelTimeout.Event:
				return fmt.Errorf("el QR caducó sin escanearse; vuelve a arrancar el bot")
			default:
				if item.Error != nil {
					return fmt.Errorf("vinculación por QR: %w", item.Error)
				}
				return fmt.Errorf("vinculación por QR interrumpida (%s)", item.Event)
			}
		}
	}
}

// drenarCanalQR tira los códigos QR que no se usan hasta que la
// vinculación termina. Si no se drena, el canal (8 posiciones) se llena, el
// emisor de QR se planta y el pre-login deja de rotar códigos: el bot se
// queda esperando un emparejamiento que ya no avanza.
func drenarCanalQR(ch <-chan whatsmeow.QRChannelItem) {
	for range ch {
	}
}

// Login: si ya hay sesión, conecta; sin sesión EXIGE el número y vincula
// por código de 8 letras. El QR se retiró de la consola, como en el JS:
// el bot corre en el propio móvil que llevaría la cuenta —no hay cámara
// que lo escanee— y un QR dibujado en un log es una trampa honesta.
func (c *Client) Login(ctx context.Context, phone string) error {
	if c.LoggedIn() {
		return c.waClient.Connect()
	}

	phone = NormalizarTelefono(phone)
	if phone == "" {
		return fmt.Errorf("sin sesión y sin número: la vinculación es por código (--code 521… o PAIRING_NUMBER=521…)")
	}
	// whatsmeow NO conecta él solo para emparejar («websocket not connected»):
	// primero entra el socket y luego se pide el código, pero solo cuando el
	// pre-login está LISTO. Hay dos piezas que se encargan de eso:
	//
	//	· GetQRChannel (antes de Connect) espera internamente a que el
	//	  servidor mande el primer <pair-device> y emite los códigos QR.
	//	  Sin él se pedía el código justo después del noise handshake, y
	//	  WhatsApp contestaba «info query returned status 400: bad-request».
	//	· El primer item del canal (un QR cualquiera) es la señal de «ya».
	//
	// Del canal solo interesa el primer evento: el QR no se dibuja (en el
	// móvil donde corre el bot no hay cámara que lo escanee) y los errores
	// de vinculación se siguen viendo en el resto de la consola.
	sp := consola.Spinner(os.Stdout, "conectando con WhatsApp…")
	qr, err := c.waClient.GetQRChannel(ctx)
	if err != nil && !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
		if sp != nil {
			sp.Fail("no hubo conexión")
		}
		return fmt.Errorf("preparar vinculación por código: %w", err)
	}
	if qr != nil {
		// El canal solo cierra cuando termina la vinculación; se drena en
		// segundo plano para que un QR que caduca no lo llene ni lo bloquee.
		defer func() { go drenarCanalQR(qr) }()
	}
	if err := c.waClient.Connect(); err != nil {
		if sp != nil {
			sp.Fail("no hubo conexión")
		}
		return fmt.Errorf("conectar para vincular: %w", err)
	}
	if qr != nil {
		select {
		case _, abierto := <-qr: // primer QR = pre-login listo
			if !abierto {
				c.Log.Warn("el canal de vinculación se cerró antes de tiempo; se pide el código de todos modos")
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(45 * time.Second):
			// Sin señal seguimos igual: mejor pedir el código que rendirse.
			c.Log.Warn("no llegó el evento QR a tiempo; se pide el código de todos modos")
		}
	}
	if sp != nil {
		sp.UpdateText("pidiendo tu código…")
	}
	code, err := c.waClient.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, PairDisplayName)
	if err != nil {
		if sp != nil {
			sp.Fail("no llegó el código")
		}
		return fmt.Errorf("pedir código de emparejamiento: %w", err)
	}
	if sp != nil {
		sp.Success("código listo")
	}
	PanelCodigo(os.Stdout, phone, code)
	c.Log.Info("Código pedido: esperando a que lo teclees en el teléfono…")
	return nil
}

// ─── Envíos ────────────────────────────────────────────────────────────────

// SendText respeta la cola anti-ban. priority salta el cupo de calentamiento.
func (c *Client) SendText(ctx context.Context, chat JID, text string, priority bool) error {
	return c.Queue.Do(ctx, len([]rune(text)), priority, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), &waE2E.Message{Conversation: proto.String(text)})
		return err
	})
}

// SendQueue es la cola de envío, para quien necesite espaciar varias salidas
// dentro del mismo comando (el gacha manda la imagen y luego el aviso).
func (c *Client) SendQueue() *Queue { return c.Queue }

// SendTextID manda texto y devuelve el id del mensaje enviado. Lo necesita el
// gacha: para reclamar un personaje hay que poder identificar el mensaje que
// lo anunció, y ese id solo lo da WhatsApp después de enviarlo.
func (c *Client) SendTextID(ctx context.Context, chat JID, text string, priority bool) (string, error) {
	var id string
	err := c.Queue.Do(ctx, len([]rune(text)), priority, func() error {
		resp, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), &waE2E.Message{Conversation: proto.String(text)})
		if err != nil {
			return err
		}
		id = resp.ID
		return nil
	})
	if err == nil {
		c.recuerdaEnviado(chat, id)
	}
	return id, err
}

// SendImageID sube una imagen con pie de foto y devuelve el id del mensaje.
func (c *Client) SendImageID(ctx context.Context, chat JID, data []byte, caption string, priority bool) (string, error) {
	up, err := c.waClient.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", err
	}
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		Caption:       proto.String(caption),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Mimetype:      proto.String("image/jpeg"),
	}}
	var id string
	err = c.Queue.Do(ctx, len([]rune(caption)), priority, func() error {
		resp, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		if err != nil {
			return err
		}
		id = resp.ID
		return nil
	})
	if err == nil {
		c.recuerdaEnviado(chat, id)
	}
	return id, err
}

// SendImage sube y manda una imagen con pie de foto.
func (c *Client) SendImage(ctx context.Context, chat JID, data []byte, caption string, priority bool) error {
	up, err := c.waClient.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return err
	}
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		Caption:       proto.String(caption),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Mimetype:      proto.String("image/jpeg"),
	}}
	return c.Queue.Do(ctx, len([]rune(caption)), priority, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// SendProto manda un mensaje ya construido (encuestas, catálogos, botones que
// no pasan por la capa de ui…) y devuelve su id.
//
// Existe para no duplicar la cola anti-ban en cada tipo de mensaje: todo lo que
// sale del bot pasa por `Queue.Do`, con o sin envoltorio.
func (c *Client) SendProto(ctx context.Context, chat JID, msg *waE2E.Message, priority bool) (string, error) {
	var id string
	err := c.Queue.Do(ctx, 120, priority, func() error {
		resp, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		if err != nil {
			return err
		}
		id = resp.ID
		return nil
	})
	if err == nil {
		c.recuerdaEnviado(chat, id)
	}
	return id, err
}

// SendDocument sube y manda un archivo con nombre. Lo usan `.get`, `.gitclone`
// y todo lo que devuelve un archivo en vez de texto.
func (c *Client) SendDocument(ctx context.Context, chat JID, data []byte, nombre, mimetype string, priority bool) error {
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	up, err := c.waClient.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return err
	}
	msg := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Mimetype:      proto.String(mimetype),
		FileName:      proto.String(nombre),
		Title:         proto.String(nombre),
	}}
	return c.Queue.Do(ctx, 120, priority, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// PerfilPublico devuelve lo que WhatsApp deja ver de alguien sin ser su
// contacto: nombre verificado, estado (bio), foto y si la cuenta existe.
func (c *Client) PerfilPublico(ctx context.Context, jid JID) (PublicProfile, error) {
	info, err := c.waClient.GetUserInfo(ctx, []types.JID{jid.ToNonAD().whatsmeow()})
	if err != nil {
		return PublicProfile{}, err
	}
	raw := info[jid.ToNonAD().whatsmeow()]
	profile := PublicProfile{Status: raw.Status, LID: jidFromWhatsmeow(raw.LID)}
	if raw.VerifiedName != nil && raw.VerifiedName.Details != nil {
		profile.VerifiedName = raw.VerifiedName.Details.GetVerifiedName()
	}
	return profile, nil
}

// FotoPerfil devuelve los bytes de la foto de perfil, o error si no tiene (o si
// la tiene privada para este número).
func (c *Client) FotoPerfil(ctx context.Context, jid JID, previa bool) ([]byte, error) {
	info, err := c.waClient.GetProfilePictureInfo(ctx, jid.ToNonAD().whatsmeow(), &whatsmeow.GetProfilePictureParams{Preview: previa})
	if err != nil {
		return nil, err
	}
	if info == nil || info.URL == "" {
		return nil, errors.New("esa persona no tiene foto de perfil visible")
	}
	// La URL de la foto es un enlace normal (así lo documenta whatsmeow), pero
	// se baja con plazo y tope: es una descarga de red y no puede quedarse
	// colgada ni traerse 200 MB.
	return bajarURL(ctx, info.URL, 8<<20)
}

// bajarURL descarga con plazo y tope de tamaño. Se usa para las fotos de perfil
// (whatsmeow devuelve una URL normal, no un mensaje descargable).
func bajarURL(ctx context.Context, u string, max int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("el servidor respondió %s", resp.Status)
	}
	datos, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(datos)) > max {
		return nil, fmt.Errorf("la imagen pasa de %d MB", max>>20)
	}
	return datos, nil
}

// ExisteEnWhatsApp dice si un número está registrado, consultando el servidor
// (no la libreta del bot).
func (c *Client) ExisteEnWhatsApp(ctx context.Context, telefono string) (JID, bool, error) {
	res, err := c.waClient.IsOnWhatsApp(ctx, []string{telefono})
	if err != nil {
		return JID{}, false, err
	}
	for _, r := range res {
		if r.IsIn {
			return jidFromWhatsmeow(r.JID), true, nil
		}
	}
	return JID{}, false, nil
}

// Presence: "available" / "unavailable" / "composing" (como el JS).
func (c *Client) SetTyping(ctx context.Context, chat JID) {
	_ = c.waClient.SendChatPresence(ctx, chat.whatsmeow(), types.ChatPresenceComposing, types.ChatPresenceMediaText)
}

// Config devuelve la configuración con la que se creó el cliente (los
// comandos la necesitan para .owner, .info, etc.).
func (c *Client) Config() config.Config { return c.cfg }

// Prefixes / FirstPrefix: el router y los comandos leen los prefijos del .env.
func (c *Client) Prefixes() []string { return c.cfg.Prefixes }

// rutaQR: donde se guarda el QR en imagen cuando el terminal es demasiado
// estrecho para dibujarlo (Termux en vertical). Va en el StateDir porque es
// donde el usuario ya tiene lo demás (auth.db, logs).
func (c *Client) rutaQR() string {
	if c.cfg.StateDir == "" {
		return "vincular-qr.png"
	}
	return filepath.Join(c.cfg.StateDir, "vincular-qr.png")
}

func (c *Client) FirstPrefix() string {
	if len(c.cfg.Prefixes) == 0 {
		return "."
	}
	return c.cfg.Prefixes[0]
}
