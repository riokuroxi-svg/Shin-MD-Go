// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// utilidades.go — Puerto de cmds/utils/*.js (primera tanda: las que no
// dependen de ninguna API ajena).
//
//	.qr          QR dibujado en el binario (el JS usaba api.qrserver.com)
//	.morse       texto → morse        .demorse  morse → texto
//	.recordar    recordatorio que sobrevive a un reinicio
//	.say         repetir un mensaje como el bot
//	.encuesta    encuesta, quiz y voto múltiple nativos
//	.pfp         foto de perfil de alguien
//	.wastalk     información pública de un número
//	.readviewonce ver un mensaje de «ver una vez»
//	.get         pedir una URL (con filtro anti-SSRF)
//
// Las que necesitan una API de terceros (IA, letra, traducir, acortar, precio,
// carbon, tourl, voz) van en otra tanda, después de comprobar que las APIs
// siguen vivas: una utilidad que miente es peor que una que no existe.
package cmds

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"shin-go/internal/bot"
	"shin-go/internal/store"
	"shin-go/internal/utilidades"
	"shin-go/internal/wa"
)

func init() {
	registerQR()
	registerMorse()
	registerRecordar()
	registerOlvidar()
	registerSay()
	registerEncuesta()
	registerPFP()
	registerWAStalk()
	registerReadViewOnce()
	registerGet()

	// Vigilante de recordatorios: uno solo para todo el proceso.
	go vigilarRecordatorios()
}

// ─── .qr ────────────────────────────────────────────────────────────────────

func registerQR() {
	bot.Register(bot.Command{
		Name:     "qr",
		Aliases:  []string{"qrcode", "qrgen", "makeqr"},
		Category: bot.CatUtils,
		Desc:     "Generar un código QR a partir de un texto o enlace",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			texto := strings.TrimSpace(c.Text)
			if texto == "" {
				return c.Reply(fmt.Sprintf(
					"《✧》 Escríbeme el *texto* o *enlace* para el QR.\n> Ejemplo: %sqr https://github.com/riokuroxi-svg/Shin-MD-Go",
					c.Bot.FirstPrefix()))
			}
			datos, err := utilidades.ImagenQR(texto)
			if err != nil {
				return c.Reply("《✧》 No pude generar el QR.\n> " + bot.Esc(err.Error()))
			}
			// El contenido del QR va como pie de foto recortado: si es una
			// contraseña, al menos no se repite entera en el chat.
			pie := "📱 *QR generado*\n> Contenido: " + bot.Esc(recortar(texto, 120))
			return c.ReplyImage(datos, "", pie, "")
		},
	})
}

// ─── .morse / .demorse ──────────────────────────────────────────────────────

func registerMorse() {
	bot.Register(bot.Command{
		Name:     "morse",
		Aliases:  []string{"amorse", "codificarmorse"},
		Category: bot.CatUtils,
		Desc:     "Convertir texto a código morse",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			texto := strings.TrimSpace(c.Text)
			if texto == "" {
				return c.Reply(fmt.Sprintf("《✧》 Escríbeme el texto.\n> Ejemplo: %smorse hola mundo\n> Para volver: %sdemorse .... --- .-.. .- / -- ..- -. -.. ---",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
			codigo, fuera := utilidades.ASimbolo(texto)
			if codigo == "" {
				return c.Reply("《✧》 De eso no pude sacar ni un símbolo morse.")
			}
			salida := "📡 *CÓDIGO MORSE*\n\n```" + codigo + "```\n\n> Para volver: *" +
				c.Bot.FirstPrefix() + "demorse " + codigo + "*"
			if fuera > 0 {
				salida += fmt.Sprintf("\n> ⚠ %d carácter(es) no se pudieron convertir (tildes, emoji, ñ…).", fuera)
			}
			return c.Reply(salida)
		},
	})

	bot.Register(bot.Command{
		Name:     "demorse",
		Category: bot.CatUtils,
		Desc:     "Convertir código morse a texto",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			texto := strings.TrimSpace(c.Text)
			if texto == "" {
				return c.Reply("《✧》 Escríbeme el código morse.\n> Ejemplo: " + c.Bot.FirstPrefix() +
					"demorse .... --- .-.. .- / -- ..- -. -.. ---")
			}
			plano, fuera := utilidades.ATexto(texto)
			if plano == "" {
				return c.Reply("《✧》 Eso no parece código morse.")
			}
			salida := "📡 *MENSAJE*\n\n> " + bot.Esc(plano)
			if fuera > 0 {
				salida += fmt.Sprintf("\n> ⚠ %d grupo(s) no se entendieron y salieron como «?».", fuera)
			}
			return c.Reply(salida)
		},
	})
}

// ─── .recordar ──────────────────────────────────────────────────────────────

const maxRecordatorio = 24 * time.Hour

func registerRecordar() {
	bot.Register(bot.Command{
		Name:     "recordar",
		Aliases:  []string{"recordatorio", "reminder", "alarma"},
		Category: bot.CatUtils,
		Desc:     "Programar un recordatorio en este chat (.recordar 5|tomar agua)",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			texto := strings.TrimSpace(c.Text)
			if strings.EqualFold(firstArg(c), "lista") {
				return listaRecordatorios(c)
			}
			if texto == "" {
				return c.Reply(fmt.Sprintf(
					"《✧》 Programa un recordatorio.\n> Formato: %srecordar <minutos>|<mensaje>\n> Ejemplo: %srecordar 5|tomar agua\n> Ver los tuyos: %srecordar lista",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
			sep := strings.Index(texto, "|")
			if sep < 0 {
				return c.Reply("《✧》 Separa los minutos del mensaje con *|*.\n> Ejemplo: " +
					c.Bot.FirstPrefix() + "recordar 5|tomar agua")
			}
			minutos, err := strconv.Atoi(strings.TrimSpace(texto[:sep]))
			if err != nil || minutos < 1 {
				return c.Reply("《✧》 Los minutos tienen que ser un número mayor que 0.")
			}
			mensaje := strings.TrimSpace(texto[sep+1:])
			if mensaje == "" {
				return c.Reply("《✧》 Falta el mensaje del recordatorio.")
			}
			if len([]rune(mensaje)) > 300 {
				mensaje = recortar(mensaje, 300)
			}

			espera := time.Duration(minutos) * time.Minute
			aviso := ""
			if espera > maxRecordatorio {
				espera = maxRecordatorio
				aviso = "\n> (el máximo son 24 h: lo programé para dentro de un día)"
			}

			cuando := time.Now().Add(espera)
			if _, err := c.DB.PonRecordatorio(c.Ctx, c.Chat.String(), c.Key(), mensaje, cuando); err != nil {
				return c.Reply("⚠️ No pude guardar el recordatorio: " + err.Error())
			}
			return c.ReplyButtons(
				"⏰ Recordatorio programado",
				fmt.Sprintf("📝 » %s\n\n> Te aviso en *%s* (a las %s).%s",
					bot.Esc(mensaje), formatWait(int64(espera.Seconds())), cuando.Format("15:04"), aviso),
				"Aunque el bot se reinicie, el aviso sigue guardado",
				bot.QuickReply("📋 Mis recordatorios", "recordar lista"))
		},
	})
}

// registerOlvidar borra un recordatorio propio por su número.
//
// El listado promete este comando, así que tiene que existir: nadie debe
// quedarse con un aviso programado que ya no quiere.
func registerOlvidar() {
	bot.Register(bot.Command{
		Name:     "olvidar",
		Aliases:  []string{"delrecordatorio", "borrarrecordatorio"},
		Category: bot.CatUtils,
		Desc:     "Borrar un recordatorio tuyo (.olvidar <número>)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			arg := strings.TrimPrefix(strings.TrimSpace(firstArg(c)), "#")
			id, err := strconv.ParseInt(arg, 10, 64)
			if err != nil || id <= 0 {
				return c.Reply("《✧》 Dime cuál.\n> Mira los números con *" +
					c.Bot.FirstPrefix() + "recordar lista*")
			}
			if err := c.DB.BorrarRecordatorio(c.Ctx, id, c.Key()); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return c.Reply("《✧》 No existe ningún recordatorio tuyo con ese número.")
				}
				return c.Reply("⚠️ No pude borrarlo: " + err.Error())
			}
			return c.Reply(fmt.Sprintf("🗑️ Recordatorio *#%d* borrado.", id))
		},
	})
}

func listaRecordatorios(c *bot.Context) error {
	pendientes, err := c.DB.RecordatoriosDe(c.Ctx, c.Chat.String(), c.Key())
	if err != nil {
		return c.Reply("⚠️ No pude leer tus recordatorios.")
	}
	if len(pendientes) == 0 {
		return c.Reply("ꕥ No tienes recordatorios pendientes en este chat.")
	}
	var b strings.Builder
	b.WriteString("⏰ *Tus recordatorios*\n")
	for i, r := range pendientes {
		if i >= 10 {
			fmt.Fprintf(&b, "│ … y %d más\n", len(pendientes)-10)
			break
		}
		fmt.Fprintf(&b, "│ #%d · en %s · %s\n", r.ID,
			formatWait(int64(time.Until(r.Cuando).Seconds())), bot.Esc(recortar(r.Texto, 60)))
	}
	b.WriteString("\n> Borrar uno: *" + c.Bot.FirstPrefix() + "olvidar <número>*")
	return c.Reply(b.String())
}

// vigilarRecordatorios revisa la base cada 20 s y avisa lo que ya toca.
//
// Es un bucle y no un `time.Timer` por recordatorio: los timers viven en
// memoria y se pierden al reiniciar, que es justo el fallo que se está
// arreglando. Además un solo bucle no puede quedarse sin gorutinas.
func vigilarRecordatorios() {
	// Arranca con un respiro: el bot acaba de conectar y no conviene empezar
	// mandando mensajes antes de que la sesión esté lista.
	time.Sleep(30 * time.Second)
	for {
		avisarVencidos()
		time.Sleep(20 * time.Second)
	}
}

func avisarVencidos() {
	ref := recordatoriosRef()
	if ref == nil {
		return // el bot todavía no arrancó del todo
	}
	vencidos, err := ref.db.RecordatoriosVencidos(ref.ctx, time.Now())
	if err != nil {
		ref.log.Warn("no se pudieron leer los recordatorios: " + err.Error())
		return
	}
	for _, r := range vencidos {
		chat, err := types.ParseJID(r.ChatID)
		if err != nil {
			continue
		}
		hace := formatWait(int64(time.Since(r.Creado).Seconds()))
		texto := fmt.Sprintf("⏰ *Recordatorio*\n\n» %s\n\n_(programado hace %s)_",
			bot.Esc(r.Texto), hace)
		// Prioridad: un recordatorio llega tarde por definición; no puede
		// quedarse esperando el cupo del calentamiento.
		if err := ref.bot.SendText(ref.ctx, chat, texto, true); err != nil {
			ref.log.Warn("no se pudo mandar un recordatorio: " + err.Error())
		}
	}
}

// referencia es lo que necesita el vigilante: base, cliente y logger.
type referencia struct {
	db  *store.Store
	bot *wa.Client
	log interface{ Warn(string) }
	ctx context.Context
}

var referenciaGlobal *referencia

// SetReferencia la llama cmd/shin al arrancar (o el bot al registrar el primer
// comando). Sin ella el vigilante no hace nada, en vez de caerse.
func SetReferencia(db *store.Store, cliente *wa.Client, log interface{ Warn(string) }, ctx context.Context) {
	referenciaGlobal = &referencia{db: db, bot: cliente, log: log, ctx: ctx}
}

func recordatoriosRef() *referencia { return referenciaGlobal }

// ─── .say ───────────────────────────────────────────────────────────────────

func registerSay() {
	bot.Register(bot.Command{
		Name:     "say",
		Aliases:  []string{"decir"},
		Category: bot.CatUtils,
		Desc:     "Repetir un mensaje como el bot (.say hola, o citando un mensaje)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			texto := strings.TrimSpace(c.Text)

			// Sin texto: si están citando algo, se repite el texto citado.
			if texto == "" {
				if citado := textoCitado(c); citado != "" {
					texto = citado
				}
			}
			if texto == "" {
				return c.Reply("《✧》 Escríbeme qué quieres que diga.\n> Ejemplo: " +
					c.Bot.FirstPrefix() + "say hola a todos")
			}
			// El texto es de quien escribe, pero el mensaje lo firma el bot:
			// se limpia el formato para que no pueda falsificar un aviso.
			return c.Reply(bot.Esc(recortar(texto, 2000)))
		},
	})
}

// textoCitado saca el texto del mensaje al que responden, si lo hay.
func textoCitado(c *bot.Context) string {
	if c.Event == nil {
		return ""
	}
	ci := wa.ContextInfo(wa.Unwrap(c.Event.Message))
	if ci == nil || ci.GetQuotedMessage() == nil {
		return ""
	}
	citado := wa.Unwrap(ci.GetQuotedMessage())
	switch {
	case citado.GetConversation() != "":
		return citado.GetConversation()
	case citado.GetExtendedTextMessage() != nil:
		return citado.GetExtendedTextMessage().GetText()
	case citado.GetImageMessage() != nil:
		return citado.GetImageMessage().GetCaption()
	case citado.GetVideoMessage() != nil:
		return citado.GetVideoMessage().GetCaption()
	case citado.GetDocumentMessage() != nil:
		return citado.GetDocumentMessage().GetCaption()
	}
	return ""
}

// ─── .encuesta ──────────────────────────────────────────────────────────────

func registerEncuesta() {
	bot.Register(bot.Command{
		Name:     "encuesta",
		Aliases:  []string{"poll", "votacion", "quiz"},
		Category: bot.CatUtils,
		Desc:     "Encuesta, quiz o voto múltiple (.encuesta ¿Pizza?|Pepperoni|Hawaiana)",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			e, err := utilidades.ParsearEncuesta(c.Text)
			if err != nil {
				return c.Reply("《✧》 " + err.Error() + "\n\n> Quiz: marca la buena con *asterisco*\n> Varias respuestas: escribe *multi* delante")
			}
			msg := construirEncuesta(e)
			if _, err := c.Bot.SendProto(c.Ctx, c.Chat, msg, c.Priority()); err != nil {
				return c.Reply("✕ No pude mandar la encuesta: " + bot.Esc(err.Error()))
			}
			return nil
		},
	})
}

// construirEncuesta arma el protobuf. Se separa para poder probarlo sin red.
//
// El MessageSecret es obligatorio: sin él, los votos no se pueden descifrar y
// la encuesta se ve pero no cuenta nada.
func construirEncuesta(e *utilidades.Encuesta) *waE2E.Message {
	opciones := make([]*waE2E.PollCreationMessage_Option, len(e.Opciones))
	for i, o := range e.Opciones {
		opciones[i] = &waE2E.PollCreationMessage_Option{OptionName: proto.String(o)}
	}
	creacion := &waE2E.PollCreationMessage{
		Name:                   proto.String(e.Pregunta),
		Options:                opciones,
		SelectableOptionsCount: proto.Uint32(uint32(e.Seleccionables)),
	}
	if e.Tipo == utilidades.TipoQuiz && e.Correcta >= 0 && e.Correcta < len(opciones) {
		creacion.PollType = waE2E.PollType_QUIZ.Enum()
		creacion.CorrectAnswer = opciones[e.Correcta]
	}
	return &waE2E.Message{
		PollCreationMessage: creacion,
		MessageContextInfo: &waE2E.MessageContextInfo{
			MessageSecret: secretoAleatorio(),
		},
	}
}

// secretoAleatorio son 32 bytes de crypto/rand. Si fallara (no debería), se
// devuelve nil y la encuesta saldría sin poder contar votos: por eso se comprueba
// con una prueba.
func secretoAleatorio() []byte {
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		return nil
	}
	return b
}

// ─── .pfp ───────────────────────────────────────────────────────────────────

func registerPFP() {
	bot.Register(bot.Command{
		Name:     "pfp",
		Aliases:  []string{"getpic", "fotoperfil"},
		Category: bot.CatUtils,
		Desc:     "Ver la foto de perfil de alguien a quien menciones o cites",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			quien, ok := personaObjetivo(c)
			if !ok {
				return c.Reply("《✧》 Etiqueta o cita a la persona cuya foto quieres ver.")
			}
			datos, err := c.Bot.FotoPerfil(c.Ctx, quien, false)
			if err != nil {
				return c.Reply(fmt.Sprintf("《✧》 No pude ver la foto de *%s*.\n> Puede que no tenga o que la tenga privada.",
					corto(quien.String())))
			}
			return c.ReplyImage(datos, "", "", "")
		},
	})
}

// ─── .wastalk ───────────────────────────────────────────────────────────────

func registerWAStalk() {
	bot.Register(bot.Command{
		Name:     "wastalk",
		Aliases:  []string{"stalknum", "numbstalk", "perfilwa", "infowa"},
		Category: bot.CatUtils,
		Desc:     "Información pública de un número de WhatsApp",
		Cooldown: 15,
		Run: func(c *bot.Context) error {
			jid, ok := personaObjetivo(c)
			if !ok {
				// También se puede escribir el número a pelo.
				digitos := soloDigitos(c.Text)
				if len(digitos) < 8 {
					return c.Reply(fmt.Sprintf(
						"《✧》 Etiqueta, cita o escribe el *número* a consultar.\n> Ejemplos:\n> • %swastalk @usuario\n> • %swastalk 5215512345678",
						c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
				}
				encontrado, existe, err := c.Bot.ExisteEnWhatsApp(c.Ctx, digitos)
				if err != nil {
					return c.Reply("《✧》 No pude consultar el número ahora mismo.")
				}
				if !existe {
					return c.Reply("《✧》 El número *" + digitos + "* no está registrado en WhatsApp.")
				}
				jid = encontrado
			}

			info, err := c.Bot.PerfilPublico(c.Ctx, jid)
			if err != nil {
				return c.Reply("《✧》 No pude leer el perfil de ese número.")
			}

			lineas := []string{
				"《⊛》 *Perfil público*",
				"│ ◈ Número › *+" + bot.Esc(jid.User) + "*",
			}
			if nombre := nombreVerificado(info.VerifiedName); nombre != "" {
				lineas = append(lineas, "│ ◈ Verificado › *"+bot.Esc(nombre)+"*")
			}
			if info.Status != "" {
				lineas = append(lineas, "│ ◈ Estado › _"+bot.Esc(recortar(info.Status, 140))+"_")
			} else {
				lineas = append(lineas, "│ ◈ Estado › _(sin biografía)_")
			}
			if info.LID.User != "" {
				lineas = append(lineas, "│ ◈ LID › _"+bot.Esc(info.LID.User)+"_")
			}

			// La foto necesita otra consulta y puede no existir: si falla, se
			// dice y se manda el texto igual.
			foto, errFoto := c.Bot.FotoPerfil(c.Ctx, jid, true)
			if errFoto == nil && len(foto) > 0 {
				pie := strings.Join(append(lineas, "╰─ foto de perfil"), "\n")
				return c.ReplyImage(foto, "", pie, "")
			}
			lineas = append(lineas, "│ ◈ Foto › _(no visible)_", "╰─")
			return c.Reply(strings.Join(lineas, "\n"))
		},
	})
}

// ─── .readviewonce ──────────────────────────────────────────────────────────

func registerReadViewOnce() {
	bot.Register(bot.Command{
		Name:     "readviewonce",
		Aliases:  []string{"read", "readvo", "verveces"},
		Category: bot.CatUtils,
		Desc:     "Ver un mensaje de «ver una vez» que te hayan mandado o citado",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			// wa.MedioDe ya desenvuelve ViewOnce/ViewOnceV2 y también mira el
			// mensaje citado, así que aquí no hay nada especial que hacer.
			medio, ok := wa.MedioDe(c.Event.Message)
			if !ok {
				return c.Reply(fmt.Sprintf(
					"《✧》 Cita un mensaje de *ver una vez* (foto, vídeo o audio) y escribe *%sread*.",
					c.Bot.FirstPrefix()))
			}
			datos, err := c.Bot.DownloadMedia(c.Ctx, medio.Data)
			if err != nil {
				return c.Reply("《✧》 No pude descargar ese mensaje: puede que ya se haya borrado.")
			}
			if len(datos) == 0 {
				return c.Reply("《✧》 El archivo llegó vacío.")
			}
			switch medio.Tipo {
			case wa.MedioImagen:
				return c.ReplyImage(datos, "", "👁️ _Mensaje de ver una vez_", "")
			case wa.MedioVideo:
				return c.Bot.SendDocument(c.Ctx, c.Chat, datos, "ver-una-vez.mp4", "video/mp4", c.Priority())
			case wa.MedioAudio:
				return c.Bot.SendDocument(c.Ctx, c.Chat, datos, "ver-una-vez.ogg", "audio/ogg", c.Priority())
			default:
				return c.Bot.SendDocument(c.Ctx, c.Chat, datos, "ver-una-vez.bin", "", c.Priority())
			}
		},
	})
}

// ─── .get ───────────────────────────────────────────────────────────────────

func registerGet() {
	bot.Register(bot.Command{
		Name:      "get",
		Aliases:   []string{"fetch", "pedir"},
		Category:  bot.CatUtils,
		Desc:      "Pedir una URL y devolver el texto o el archivo",
		OwnerOnly: true, // puede alcanzar la red del servidor: solo el dueño
		Cooldown:  10,
		Run: func(c *bot.Context) error {
			enlace := strings.TrimSpace(firstArg(c))
			if enlace == "" {
				return c.Reply("《✧》 Escríbeme el enlace.\n> Ejemplo: " + c.Bot.FirstPrefix() +
					"get https://api.github.com/repos/riokuroxi-svg/Shin-MD-Go")
			}
			u, err := utilidades.ValidarURL(enlace, utilidades.ResolverDNSIPs)
			if err != nil {
				return c.Reply("🚫 " + bot.Esc(err.Error()))
			}

			ctx, cancel := context.WithTimeout(c.Ctx, 20*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return c.Reply("《✧》 Ese enlace no se entiende.")
			}
			req.Header.Set("User-Agent", "Shin-MD-Go (+https://github.com/riokuroxi-svg/Shin-MD-Go)")
			resp, err := clienteHTTP.Do(req)
			if err != nil {
				return c.Reply("《✧》 No pude conectar con ese enlace.")
			}
			defer func() { _ = resp.Body.Close() }()

			datos, err := io.ReadAll(io.LimitReader(resp.Body, utilidades.MaxDescarga+1))
			if err != nil {
				return c.Reply("《✧》 Se cortó la descarga.")
			}
			if int64(len(datos)) > utilidades.MaxDescarga {
				return c.Reply(fmt.Sprintf("《✧》 Ese archivo pasa de %d MB: no lo voy a descargar entero.",
					utilidades.MaxDescarga>>20))
			}

			tipo := resp.Header.Get("Content-Type")
			switch {
			case esTexto(tipo):
				return c.Reply("《⊛》 *" + bot.Esc(u.Host) + "* · " + resp.Status + "\n```" +
					utilidades.RecortarParaChat(string(datos)) + "```")
			case strings.HasPrefix(tipo, "image/"):
				return c.ReplyImage(datos, "", bot.Esc(u.Host), "")
			default:
				nombre := nombreDeArchivo(u.Path, tipo)
				return c.Bot.SendDocument(c.Ctx, c.Chat, datos, nombre, tipo, c.Priority())
			}
		},
	})
}

var clienteHTTP = &http.Client{
	Timeout: 25 * time.Second,
	// Sin seguir redirecciones a ciegas: una redirección es justo el truco para
	// saltarse el filtro de IPs privadas (te manda a 127.0.0.1 desde un dominio
	// público). Si hay redirección, se dice y que la pidan a mano.
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// esTexto decide si la respuesta se puede mandar como texto.
func esTexto(tipo string) bool {
	tipo = strings.ToLower(tipo)
	for _, prefijo := range []string{"text/", "application/json", "application/xml", "application/javascript"} {
		if strings.HasPrefix(tipo, prefijo) {
			return true
		}
	}
	return false
}

func nombreDeArchivo(ruta, tipo string) string {
	if i := strings.LastIndexByte(ruta, '/'); i >= 0 && i+1 < len(ruta) {
		if n := ruta[i+1:]; n != "" {
			return n
		}
	}
	switch {
	case strings.HasPrefix(tipo, "image/"):
		return "archivo.png"
	case strings.HasPrefix(tipo, "application/pdf"):
		return "archivo.pdf"
	case strings.HasPrefix(tipo, "audio/"):
		return "archivo.ogg"
	case strings.HasPrefix(tipo, "video/"):
		return "archivo.mp4"
	}
	return "archivo.bin"
}

// ─── ayudas ─────────────────────────────────────────────────────────────────

// personaObjetivo resuelve de quién se habla: la primera mención, el autor del
// mensaje citado, o (si no) nadie.
func personaObjetivo(c *bot.Context) (types.JID, bool) {
	if menciones := c.Mentioned(); len(menciones) > 0 {
		return menciones[0].ToNonAD(), true
	}
	if c.Event == nil {
		return types.JID{}, false
	}
	ci := wa.ContextInfo(wa.Unwrap(c.Event.Message))
	if ci == nil {
		return types.JID{}, false
	}
	participante := ci.GetParticipant()
	if participante == "" {
		return types.JID{}, false
	}
	jid, err := types.ParseJID(participante)
	if err != nil {
		return types.JID{}, false
	}
	return jid.ToNonAD(), true
}

// nombreVerificado saca el nombre de empresa verificado, si lo tiene.
//
// whatsmeow lo guarda dentro del certificado (`Details.VerifiedName`), no en un
// campo directo: mirar `VerifiedName != nil` no basta, porque el certificado
// puede venir sin detalles.
func nombreVerificado(v *types.VerifiedName) string {
	if v == nil || v.Details == nil {
		return ""
	}
	return strings.TrimSpace(v.Details.GetVerifiedName())
}

// soloDigitos deja el número limpio de todo lo que no sea dígito.
func soloDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// recortar corta por runas (no por bytes: cortar un emoji por la mitad deja un
// carácter inválido que WhatsApp rechaza).
func recortar(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
