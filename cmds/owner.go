// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// owner.go — Puerto de cmds/owner/*.js: los comandos que tocan el proceso y el
// disco. Todos son solo del dueño y todos tienen algo que el JS hacía mal.
//
// LO QUE SE PORTA Y LO QUE NO
//
//	.restart   sí. En el JS se moría si no había supervisor; aquí se mira
//	           quién nos sostiene antes de salir (ver reiniciar()).
//	.update    sí, con `git pull --ff-only` y sin recompilar a ciegas.
//	.subir     sí, con el mismo candado anti-escape del JS.
//	.cache     sí: ver y limpiar los temporales.
//	.setmenu   sí, con las variantes que este bot puede pintar de verdad.
//	.r / .shell sí, pero APAGADO por defecto: ejecutar comandos del sistema
//	           desde el chat es abrir la puerta de casa y dejar la llave puesta.
//	           Se enciende con OWNER_SHELL=1 en el .env.
//	.ex / .e   NO. Ejecutaban JavaScript dentro del bot; en un binario Go no
//	           hay nada equivalente, y montar un intérprete para eso sería
//	           peor que no tenerlo.
package cmds

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"shin-go/internal/bot"
)

func init() {
	registerRestart()
	registerUpdate()
	registerSubir()
	registerShell()
	registerCache()
	registerSetMenu()
}

// ─── .restart ───────────────────────────────────────────────────────────────

// caminoDeReinicio responde a una sola pregunta: ¿quién me vuelve a levantar
// cuando salga? Se aísla para poder probarlo sin reiniciar nada.
//
//	"systemd"  → hay un supervisor (systemd, docker restart, runit)
//	"exec"     → nadie nos sostiene, pero podemos relanzar el binario nosotros
//	"sin-salida" → si salimos, el bot se queda apagado
func caminoDeReinicio(haySupervisor, puedeExec bool) string {
	if haySupervisor {
		return "systemd"
	}
	if puedeExec {
		return "exec"
	}
	return "sin-salida"
}

// haySupervisor detecta si el proceso está bajo algo que lo reinicia.
//
// No se adivina: systemd exporta INVOCATION_ID a sus unidades; docker y los
// demás se declaran con SHIN_SUPERVISOR=1 cuando el usuario quiere.
func haySupervisor() bool {
	return os.Getenv("INVOCATION_ID") != "" || os.Getenv("SHIN_SUPERVISOR") == "1"
}

func registerRestart() {
	bot.Register(bot.Command{
		Name:      "restart",
		Aliases:   []string{"reiniciar"},
		Category:  bot.CatOwner,
		Desc:      "Reiniciar el bot (solo si hay algo que lo vuelva a levantar)",
		OwnerOnly: true,
		Priority:  true,
		Cooldown:  30,
		Run: func(c *bot.Context) error {
			_, err := os.Executable()
			puedeExec := err == nil && os.Getenv("SHIN_REINICIO") == "exec"
			via := caminoDeReinicio(haySupervisor(), puedeExec)

			if via == "sin-salida" {
				// Morirse aquí deja el bot apagado hasta que alguien lo
				// note. En el JS pasaba exactamente eso con `npm start`.
				return c.Reply("「✧」 No puedo reiniciarme en esta instalación.\n" +
					"> No hay supervisor (systemd/docker) que me vuelva a levantar.\n" +
					"> Si me reiniciara ahora, el bot quedaría apagado.\n" +
					"> · Con systemd: funciona solo.\n" +
					"> · A mano: pon `SHIN_REINICIO=exec` en el .env y lo relanzo yo.")
			}

			mensaje := "「✧」 Reiniciando…\n> *Espera un momento.*"
			if via == "exec" {
				mensaje += "\n> (me relanzo yo; no hay supervisor)"
			}
			if err := c.ReplyPriority(mensaje); err != nil {
				return err
			}

			// Se espera a que salga el mensaje: si se sale en el acto, el
			// propio reinicio se lleva por delante el envío pendiente.
			go reiniciar(via)
			return nil
		},
	})
}

// reiniciar espera a que la cola vacíe y entonces sale (o se relanza).
func reiniciar(via string) {
	time.Sleep(3 * time.Second)
	if via == "exec" {
		exe, err := os.Executable()
		if err != nil {
			return // sin binario no se sale: muerto no sirve de nada
		}
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()
		if err := cmd.Start(); err != nil {
			return // el hijo no arrancó: seguir vivo es lo único sensato
		}
	}
	os.Exit(0)
}

// ─── .update ────────────────────────────────────────────────────────────────

func registerUpdate() {
	bot.Register(bot.Command{
		Name:      "update",
		Aliases:   []string{"fix"},
		Category:  bot.CatOwner,
		Desc:      "Actualizar el código con git pull (y decir qué hacer después)",
		OwnerOnly: true,
		Cooldown:  60,
		Run: func(c *bot.Context) error {
			raiz, err := raizDelBot()
			if err != nil {
				return c.Reply("「✧」 No sé dónde está el código: " + err.Error())
			}
			if _, err := os.Stat(filepath.Join(raiz, ".git")); err != nil {
				return c.Reply("「✧」 Esto no es un repositorio git, así que no hay nada que actualizar.\n" +
					"> Si instalaste con `git clone`, ejecútame desde esa carpeta.")
			}

			ctx, cancel := context.WithTimeout(c.Ctx, 60*time.Second)
			defer cancel()
			// --ff-only: si hay cambios locales, git se niega en vez de
			// mezclar cosas a medias. Un bot a medio mezclar no arranca.
			salida, err := exec.CommandContext(ctx, "git", "-C", raiz, "pull", "--ff-only").CombinedOutput()
			texto := bot.ClipText(strings.TrimSpace(string(salida)), 1200)

			if err != nil {
				return c.Reply("「✧」 El `git pull` no terminó bien:\n> " + texto +
					"\n> (suele ser un cambio local sin commitear)")
			}
			if strings.Contains(texto, "Already up to date") || strings.Contains(texto, "Ya está actualizado") {
				return c.Reply("ꕥ *Todo actualizado.*\n> " + texto)
			}
			return c.ReplyButtons(
				"ꕥ *Código actualizado*",
				texto+"\n\n> Falta recompilar: el binario no cambia solo.",
				"go build && reiniciar",
				bot.QuickReply("🔄 Reiniciar", "restart"),
				bot.QuickReply("📊 Estado", "health"))
		},
	})
}

// raizDelBot busca la carpeta del proyecto: la del .env si se conoce, y si no
// el directorio de trabajo.
func raizDelBot() (string, error) {
	if env := os.Getenv("SHIN_ROOT"); env != "" {
		return filepath.Abs(env)
	}
	if dir := filepath.Dir(cfgEnvFile()); dir != "." && dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			return abs, nil
		}
	}
	return os.Getwd()
}

func cfgEnvFile() string {
	if v := os.Getenv("SHIN_ENV_FILE"); v != "" {
		return v
	}
	return ".env"
}

// ─── .subir ─────────────────────────────────────────────────────────────────

const maxSubida = 200_000 // caracteres, igual que el JS

func registerSubir() {
	bot.Register(bot.Command{
		Name:      "subir",
		Aliases:   []string{"upload", "escribir"},
		Category:  bot.CatOwner,
		Desc:      "Escribir un archivo del bot sin tocar Termux (.subir ruta | contenido)",
		OwnerOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			raw := strings.TrimSpace(c.Text)
			if raw == "" || !strings.Contains(raw, "|") {
				return c.Reply("《✧》 Uso: *.subir ruta | contenido*\n" +
					"Ejemplo: `.subir nota.txt | hola mundo`\n" +
					"La ruta es relativa a la carpeta del bot (no puede salirse con `..`).")
			}

			sep := strings.Index(raw, "|")
			rutaPedida := strings.TrimSpace(raw[:sep])
			// Se quita UN espacio o salto tras la barra, pero nunca más: la
			// indentación del código es parte del contenido.
			contenido := strings.TrimPrefix(strings.TrimPrefix(raw[sep+1:], " "), "\n")

			if rutaPedida == "" {
				return c.Reply("《✧》 Falta la ruta. Uso: `.subir ruta | contenido`")
			}
			if len([]rune(contenido)) > maxSubida {
				return c.Reply(fmt.Sprintf("《✧》 Contenido demasiado largo (%d caracteres, máx %d).",
					len([]rune(contenido)), maxSubida))
			}

			raiz, err := raizDelBot()
			if err != nil {
				return c.Reply("「✧」 No sé dónde está el código: " + err.Error())
			}
			destino, rel, err := rutaSegura(raiz, rutaPedida)
			if err != nil {
				return c.Reply("🚫 " + err.Error())
			}
			if info, err := os.Stat(destino); err == nil && info.IsDir() {
				return c.Reply("🚫 Esa ruta es una carpeta, no un archivo.")
			}
			if err := os.MkdirAll(filepath.Dir(destino), 0o750); err != nil {
				return c.Reply("⚠️ No se pudo crear la carpeta: " + err.Error())
			}
			if err := os.WriteFile(destino, []byte(contenido), 0o640); err != nil {
				return c.Reply("⚠️ No se pudo escribir: " + err.Error())
			}

			extra := ""
			if strings.HasSuffix(rel, ".go") {
				extra = "\n💡 Es código Go: hace falta recompilar (`.update` y reiniciar)."
			}
			return c.Reply(fmt.Sprintf("✓ Actualizado: `%s` (%d caracteres)%s",
				rel, len([]rune(contenido)), extra))
		},
	})
}

// rutaSegura resuelve `pedida` dentro de `raiz` y se niega a salir de ella.
//
// Es la misma defensa del JS y no es paranoia: sin esto, un `.subir ../../.ssh/
// authorized_keys | ...` escribe donde no debe, y el comando es del dueño pero
// el mensaje puede venir de una cuenta comprometida.
func rutaSegura(raiz, pedida string) (absoluta, relativa string, err error) {
	if strings.ContainsRune(pedida, 0) {
		return "", "", fmt.Errorf("ruta inválida")
	}
	// Las rutas absolutas se rechazan en vez de reinterpretarlas: dejar que
	// "/etc/passwd" acabe escrita dentro del repo es seguro, pero enseña al
	// dueño una ruta que no es la que pidió.
	if filepath.IsAbs(pedida) || strings.HasPrefix(pedida, "/") {
		return "", "", fmt.Errorf("ruta no permitida: usa una ruta relativa a la carpeta del bot")
	}
	destino := filepath.Join(raiz, filepath.FromSlash(pedida))
	destino = filepath.Clean(destino)
	raiz = filepath.Clean(raiz)

	if destino != raiz && !strings.HasPrefix(destino, raiz+string(filepath.Separator)) {
		return "", "", fmt.Errorf("ruta no permitida: no puedes salir de la carpeta del bot")
	}
	rel, err := filepath.Rel(raiz, destino)
	if err != nil {
		return "", "", err
	}
	// Cualquier componente `.git` de la ruta queda protegido, no solo el de
	// la raíz: un `.git` anidado es un submódulo, y escribirlo rompe el repo.
	for _, parte := range strings.Split(filepath.ToSlash(rel), "/") {
		if parte == ".git" {
			return "", "", fmt.Errorf("`.git/` está protegido")
		}
	}
	return destino, filepath.ToSlash(rel), nil
}

// ─── .shell (.r) ────────────────────────────────────────────────────────────

func registerShell() {
	bot.Register(bot.Command{
		Name:      "shell",
		Aliases:   []string{"r", "sh"},
		Category:  bot.CatOwner,
		Desc:      "Ejecutar un comando del sistema (requiere OWNER_SHELL=1)",
		OwnerOnly: true,
		Cooldown:  5,
		Run: func(c *bot.Context) error {
			if os.Getenv("OWNER_SHELL") != "1" {
				return c.Reply("「✧」 El shell está apagado.\n" +
					"> Para encenderlo: `OWNER_SHELL=1` en el .env y reiniciar.\n" +
					"> Está apagado por defecto porque cualquiera que consiga tu\n" +
					"> cuenta puede ejecutar lo que quiera en el servidor.")
			}
			orden := strings.TrimSpace(c.Text)
			if orden == "" {
				return c.Reply("《✧》 Debes escribir un comando a ejecutar.")
			}

			ctx, cancel := context.WithTimeout(c.Ctx, 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", orden)
			salida, err := cmd.CombinedOutput()

			texto := strings.TrimSpace(string(salida))
			if texto == "" {
				texto = "(sin salida)"
			}
			texto = bot.ClipText(texto, 3000)

			if ctx.Err() != nil {
				return c.Reply("⏱️ Se cortó a los 20 s.\n```" + texto + "```")
			}
			if err != nil {
				return c.Reply("✕ Terminó con error: " + bot.Esc(err.Error()) + "\n```" + texto + "```")
			}
			return c.Reply("```" + texto + "```")
		},
	})
}

// ─── .cache ─────────────────────────────────────────────────────────────────

func registerCache() {
	bot.Register(bot.Command{
		Name:      "cache",
		Aliases:   []string{"clearcache", "limpiarcache"},
		Category:  bot.CatOwner,
		Desc:      "Ver y limpiar los temporales del bot (.cache clear [todo|tmp|media])",
		OwnerOnly: true,
		Priority:  true,
		Cooldown:  10,
		Run: func(c *bot.Context) error {
			dirs, err := carpetasDeCache(c)
			if err != nil {
				return c.Reply("「✧」 No pude leer las carpetas de datos: " + err.Error())
			}

			sub := strings.ToLower(firstArg(c))
			if sub == "" {
				var b strings.Builder
				b.WriteString("《⊛》 Caché del bot\n\n")
				var total int64
				var archivos int
				for _, d := range dirs {
					tam, n := medirCarpeta(d.ruta)
					total += tam
					archivos += n
					fmt.Fprintf(&b, "✦ *%s* › %s · %d archivo(s)\n", d.nombre, bytesLegibles(tam), n)
					if n == 0 {
						b.WriteString("▸ (vacía)\n")
					}
				}
				fmt.Fprintf(&b, "\n◇ Total › %s (%d archivos)\n", bytesLegibles(total), archivos)
				fmt.Fprintf(&b, "\n> Limpiar: *%scache clear todo*\n", c.Bot.FirstPrefix())
				b.WriteString("> La sesión de WhatsApp y la base de datos NO se tocan.")
				return c.Reply(b.String())
			}

			if sub != "clear" && sub != "limpiar" {
				return c.Reply(fmt.Sprintf("《✧》 Uso: *%scache* | *%scache clear [todo|tmp|media]*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}

			objetivo := strings.ToLower(strings.Join(c.Args[1:], ""))
			if objetivo == "" {
				objetivo = "todo"
			}

			var liberado int64
			borrados := 0
			for _, d := range dirs {
				if objetivo != "todo" && objetivo != d.clave {
					continue
				}
				tam, n := vaciarCarpeta(d.ruta)
				liberado += tam
				borrados += n
			}
			if borrados == 0 && objetivo != "todo" {
				return c.Reply("《✧》 No existe esa caché. Opciones: *todo*, *tmp*, *media*.")
			}
			return c.Reply(fmt.Sprintf("✅ Caché limpia.\n> Archivos borrados: *%d*\n> Espacio liberado: *%s*",
				borrados, bytesLegibles(liberado)))
		},
	})
}

type carpetaCache struct {
	nombre string
	clave  string
	ruta   string
}

// carpetasDeCache son los directorios que el bot usa para trabajar. NO incluye
// la sesión de WhatsApp ni la base: borrarlas es desvincular el número.
func carpetasDeCache(c *bot.Context) ([]carpetaCache, error) {
	base := c.Bot.Config().StateDir
	if base == "" {
		return nil, fmt.Errorf("StateDir vacío")
	}
	return []carpetaCache{
		{"temporales", "tmp", filepath.Join(base, "tmp")},
		{"medios", "media", filepath.Join(base, "media")},
		{"descargas", "descargas", filepath.Join(base, "downloads")},
	}, nil
}

// medirCarpeta suma tamaños sin seguir enlaces simbólicos (un enlace a / sería
// el fin del mundo en forma de bucle).
func medirCarpeta(ruta string) (int64, int) {
	var total int64
	var archivos int
	_ = filepath.WalkDir(ruta, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		archivos++
		return nil
	})
	return total, archivos
}

// vaciarCarpeta borra el CONTENIDO, no la carpeta: si se borra el directorio,
// el siguiente envío falla hasta que alguien se acuerde de crearlo.
func vaciarCarpeta(ruta string) (int64, int) {
	tam, n := medirCarpeta(ruta)
	entradas, err := os.ReadDir(ruta)
	if err != nil {
		return 0, 0
	}
	for _, e := range entradas {
		_ = os.RemoveAll(filepath.Join(ruta, e.Name()))
	}
	return tam, n
}

// ─── .setmenu ───────────────────────────────────────────────────────────────

// variantesDeMenu son las que este bot sabe pintar. El JS tenía cuatro; la de
// vídeo (v3) necesita generar un mp4 de cabecera y aquí no existe, así que se
// ofrece la lista real en vez de aceptar una que no haría nada.
var variantesDeMenu = []struct {
	clave  string
	id     int
	nombre string
	desc   string
}{
	{"v1", 1, "TARJETA CON BOTONES", "menú con botones nativos y categorías"},
	{"v2", 2, "TEXTO CON LISTA", "todo en un mensaje de texto, sin botones"},
	{"v4", 4, "FICHA COMPACTA", "resumen corto con las categorías y el estado"},
}

func registerSetMenu() {
	bot.Register(bot.Command{
		Name:      "setmenu",
		Aliases:   []string{"menuvariant", "menustyle", "cambiarmenu"},
		Category:  bot.CatOwner,
		Desc:      "Cambiar el estilo del menú principal",
		OwnerOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			pedida := strings.ToLower(firstArg(c))
			if pedida == "" {
				var b strings.Builder
				b.WriteString("🎨 *Estilo del menú*\n\n")
				b.WriteString("Uso: `" + c.Bot.FirstPrefix() + "setmenu <variante>`\n\n")
				for _, v := range variantesDeMenu {
					fmt.Fprintf(&b, "✦ *%s* — *%s*\n> _%s_\n\n", strings.ToUpper(v.clave), v.nombre, v.desc)
				}
				b.WriteString("> La variante de vídeo del bot de JS no existe aquí: este")
				b.WriteString(" bot no genera vídeo de cabecera.")
				return c.Reply(b.String())
			}

			var elegida int
			for _, v := range variantesDeMenu {
				if v.clave == pedida {
					elegida = v.id
				}
			}
			if elegida == 0 {
				return c.Reply(fmt.Sprintf("❌ Variante inválida. Opciones: *v1*, *v2*, *v4*.\nEjemplo: `%ssetmenu v1`",
					c.Bot.FirstPrefix()))
			}

			if err := c.DB.SetSetting(c.Ctx, claveAjustesBot, ajusteVarianteMenu, fmt.Sprint(elegida)); err != nil {
				return c.Reply("⚠️ No pude guardar la configuración: " + err.Error())
			}
			var n string
			for _, v := range variantesDeMenu {
				if v.id == elegida {
					n = v.nombre
				}
			}
			return c.Reply(fmt.Sprintf("✅ *Estilo del menú cambiado*\n\n✦ Variante *V%d: %s*", elegida, n))
		},
	})
}

// Ajustes globales del bot (no de una persona): van en `settings` con una clave
// fija, para que el estilo elegido valga en todos los chats.
const (
	claveAjustesBot    = "bot:ajustes"
	ajusteVarianteMenu = "menu_variant"
)

// varianteMenuActual devuelve la variante guardada (1 por defecto).
func varianteMenuActual(c *bot.Context) int {
	v, ok, err := c.DB.GetSetting(c.Ctx, claveAjustesBot, ajusteVarianteMenu)
	if err != nil || !ok {
		return 1
	}
	n, err := parseIntSeguro(v)
	if err != nil {
		return 1
	}
	return n
}

// parseIntSeguro no se cae con basura guardada en la base.
func parseIntSeguro(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n, err
}

// ordenDeComandos deja una lista de comandos ordenada por nombre: la usan el
// menú de texto y .allmenu, y así dos ejecuciones dan lo mismo.
func ordenDeComandos(cmds []*bot.Command) []*bot.Command {
	copia := make([]*bot.Command, len(cmds))
	copy(copia, cmds)
	sort.Slice(copia, func(i, j int) bool { return copia[i].Name < copia[j].Name })
	return copia
}
