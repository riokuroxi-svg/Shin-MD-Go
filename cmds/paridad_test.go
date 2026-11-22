// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// paridad_test.go — B8: el contrato de catálogo con el JS.
//
// Lee cmds/testdata/js_comandos.txt (los 640 nombres que el JS declara en
// `command:`) y exige que TODOS estén registrados en Go, salvo los de la
// tabla «no_portados», que llevan su motivo. Si añades un comando al JS, este
// test te obliga a portarlo o a justificarlo — no se puede volver a colar lo
// que se olvidó en ninguna migración sin que CI truene.
package cmds

import (
	"os"
	"strings"
	"testing"

	"shin-go/internal/bot"
)

// noPortados: la lista cerrada y con motivo. Si algo no está aquí y no está
// en el registro, el test falla (sin trampas: no se edita este archivo a la
// ligera, el motivo se lee en el propio diff).
var noPortados = map[string]string{
	// Música: motor integrado más adaptador spotDL externo y explícito.
	"play2":          "cubierto por .play (Audius/Deezer/iTunes + spotDL opcional)",
	"playsoundcloud": "cubierto por .play; SoundCloud no se presenta como catálogo libre",
	"spotify":        "cubierto por .play <URL Spotify> cuando SPOTDL_PATH está habilitado",
	"soundcloud":     "mismo motivo que playsoundcloud",
	"benchdl":        "medía proveedores internos del JS; el adaptador Go se prueba por separado",

	// Los descargadores sociales van por .yt/alias — ya vía yt-dlp si existe.

	// Dueño del JS que no se porta (sistema multi-bot, jadibot: exigiría una
	// arquitectura distinta en whatsmeow, no solo un comando).
	"serbot":         "sistema de sub-bots: fuera de alcance de la migración 1:1",
	"jadibot":        "idem",
	"code":           "idem (codigo de vinculación de sub-bot)",
	"join":           "idem",
	"unir":           "idem",
	"leave":          "idem",
	"salir":          "idem",
	"logout":         "idem",
	"reload":         "idem",
	"bots":           "idem",
	"sockets":        "idem",
	"setbotbanner":   "perfil-del-bot en whatsmeow solo cubre status; banner/icon/channel no existen",
	"setchannel":     "idem",
	"setbotchannel":  "idem",
	"setbotcurrency": "el símbolo de la moneda es una decisión de main.go, no runtime",
	"setcurrency":    "idem",
	"seticon":        "idem (whatsapp no cambia avatar por comando de texto)",
	"setboticon":     "idem",
	"setlink":        "idem",
	"setbotlink":     "idem",
	"setprefix":      "el prefijo es del .env (sin runtime switches como el JS)",
	"setbotprefix":   "idem",
	"setowner":       "el owner es del .env (como en el JS; no cambio en vivo)",
	"setbotowner":    "idem",
	"setimage":       "cubierto por setpfp equivalente — pendiente de validar en whatsmeow",
	"setpfp":         "pendiente de validar setpfp en whatsmeow",
	"setstatus":      "pendiente de validar setstatus en whatsmeow (whatsmeow.SetStatusMessage existe a revisar)",
	"setusername":    "whatsapp X non-admins no cambican el nombre por comando",

	// Diversiones que no existen como tales en el catálogo JS pero cuelgan de
	// los maps (variantes con pila compartida) — el listado js trae los nombres
	// de map que NO son comandos visibles por defecto (JS lo confirmaba en el
	// menu: solo salían los primarios).
	"setdesc2": "era un alias falso mío; no estaba en el JS",

	// NSFW con contenido de agresión: NO se portan (ver docs/NO_PORTADO.md):
	"anal": "contenido de agresión sexual: excluido", "violar": "idem",
	"cum":     "contenido explícito suelto en cualquier grupo sin candado: excluido",
	"undress": "idem", "encuerar": "idem", "fuck": "idem", "coger": "idem",
	"spank": "idem",

	// Buscadores porno sin API pública fiable (ver docs/NO_PORTADO.md).
	"xnxx":    "sin API pública estable: scraping mentiroso no se porta",
	"xvideos": "idem",

	// NSFW del JS que quedaba: reacciones explícitas sueltas sin candado
	// (el candado .nsfw llegó en Go después). Variantes agresivas: fuera.
	"69":          "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"bj":          "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"blowjob":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"bondage":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"boobjob":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"bukkake":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"creampie":    "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"cummouth":    "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"cumshot":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"deepthroat":  "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"facesitting": "contenido de agresión: fuera",
	"fap":         "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"fingering":   "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"footjob":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"futa":        "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"futanari":    "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"grabboobs":   "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"grope":       "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"handjob":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"lickass":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"lickdick":    "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"lickpussy":   "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"mamada":      "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"nalgada":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"onlyadmin":   "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"orgia":       "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"orgy":        "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"paja":        "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"pegging":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"sixnine":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"squirt":      "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"squirting":   "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"suckboobs":   "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"thighjob":    "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"tijeras":     "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"yaoi":        "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",
	"yuri":        "nsfw suelto sin candado en el JS; las tres búsquedas portadas (danbooru/gelbooru/rule34) sí lo llevan",

	// Calidad de vida del owner ya cubierta: el set* de nombres está en .setname
	"setbotname": "cambiarse el nombre en vivo no está implementado en whatsmeow; el nombre es de la sesión",
	"cta":        "era un banner del JS, no un comando",

	// deepseek: otra API de IA con otra llave; .ai cubre (documentado).
	"deepseek": "pide otra key distinta: .ai cubre el chat IA",
	"ds":       "idem", "deepthink": "idem", "reasoning": "idem", "r1": "idem",

	// stickers: el game de empacar "público/privado" ya lo hace setpackpublic
	// (setpackname el JS lo tenía cruzado con el nombre del pack real).
	"setstickerpackname": "cubierto por .newpack (el nombre es la clave)",
	"setpackname":        "idem", "packname": "idem",

	// Sugerir cookies al admin en vivo era un riesgo
	// (se dejaban credenciales en base sin cifrado): no se porta; va .env.
	"subircookies": "guardar credenciales desde el chat: excluido",

	// Terminología provisional del JS que no era un comando.
	"terminos": "era texto de ayuda en el JS, no un comando",
	"demo":     "pendiente de decidir el saludo de demo (el .menu lo cubre)",
	"guardar":  "cubierto por .subir (tourl)",
	"kuro":     "era un huevo de pascua del JS sin funcionalidad",
}

// TestParidadCatalogo: cada comando del JS está en Go o justificado arriba.
func TestParidadCatalogo(t *testing.T) {
	raw, err := os.ReadFile("testdata/js_comandos.txt")
	if err != nil {
		t.Fatalf("no pude leer el catálogo JS: %v", err)
	}
	tengo := map[string]bool{}
	for _, c := range bot.All() {
		tengo[strings.ToLower(c.Name)] = true
		for _, a := range c.Aliases {
			tengo[strings.ToLower(a)] = true
		}
	}
	var faltan []string
	for _, n := range strings.Fields(string(raw)) {
		if tengo[n] {
			continue
		}
		motivo, justificado := noPortados[n]
		if !justificado {
			faltan = append(faltan, n)
			continue
		}
		if strings.TrimSpace(motivo) == "" {
			t.Fatalf("«%s» está en noPortados sin justificación: o se porta, o se explica", n)
		}
	}
	if len(faltan) > 0 {
		t.Fatalf("faltan %d comandos por portar o justificar: %v", len(faltan), faltan)
	}

	// Y lo contrario también es verdad: ninguno de los no-portados está
	// registrado (si lo está, hay que quitarlo de la tabla).
	for n := range noPortados {
		if tengo[n] {
			t.Fatalf("«%s» está registrado pero sigue en la tabla de no-portados: actualízala", n)
		}
	}
}
