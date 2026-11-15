// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// traducir.go — Puerto de src/lib/translate.js (la parte pensante de `.tr`).
//
// WhatsApp estrenó traducción en 2025, pero se hace entera en el móvil, solo
// entre seis idiomas, sin campo alguno en el protocolo para dispararla desde
// fuera: aquí el bot sí aporta. El port mantiene las tres piezas del JS:
//
//  1. ResolverIdioma — la gente escribe «inglés», «ingles», «en» o «english».
//  2. DetectarIdioma — adivino propio, sin red: alfabetos que cantan solos,
//     palabras vacías por idioma y tildes que delatan. Sirve para no llamar
//     al traductor cuando el texto ya está en el idioma pedido.
//  3. Traducir — dos proveedores encadenados (Google gtx, luego MyMemory)
//     con caché FIFO: si uno se cae, el comando sigue vivo.
package apis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Tope de texto por traducción: más que esto lo corta el proveedor.
const TraducirMaxTexto = 1800

// Traducciones recordadas para no repetir la misma llamada.
const traducirMaxCache = 200

// Idiomas con nombre en español y su bandera (misma tabla que el JS).
var idiomas = map[string][2]string{
	"es": {"español", "🇪🇸"}, "en": {"inglés", "🇬🇧"}, "pt": {"portugués", "🇧🇷"},
	"fr": {"francés", "🇫🇷"}, "it": {"italiano", "🇮🇹"}, "de": {"alemán", "🇩🇪"},
	"ru": {"ruso", "🇷🇺"}, "ar": {"árabe", "🇸🇦"}, "hi": {"hindi", "🇮🇳"},
	"ja": {"japonés", "🇯🇵"}, "ko": {"coreano", "🇰🇷"}, "zh": {"chino", "🇨🇳"},
	"nl": {"neerlandés", "🇳🇱"}, "tr": {"turco", "🇹🇷"}, "pl": {"polaco", "🇵🇱"},
	"id": {"indonesio", "🇮🇩"}, "vi": {"vietnamita", "🇻🇳"}, "th": {"tailandés", "🇹🇭"},
	"uk": {"ucraniano", "🇺🇦"}, "ro": {"rumano", "🇷🇴"}, "el": {"griego", "🇬🇷"},
	"he": {"hebreo", "🇮🇱"}, "sv": {"sueco", "🇸🇪"}, "fil": {"filipino", "🇵🇭"},
	"ca": {"catalán", "🏳"}, "eu": {"euskera", "🏳"}, "gl": {"gallego", "🏳"},
	"la": {"latín", "🏛"},
}

// alias: cómo lo escribe la gente de verdad → código.
var aliasIdioma = map[string]string{
	"espanol": "es", "español": "es", "castellano": "es", "spanish": "es", "esp": "es",
	"ingles": "en", "inglés": "en", "english": "en", "ing": "en", "eng": "en",
	"portugues": "pt", "portugués": "pt", "brasileno": "pt", "brasileño": "pt", "portuguese": "pt", "br": "pt",
	"frances": "fr", "francés": "fr", "french": "fr",
	"italiano": "it", "italian": "it",
	"aleman": "de", "alemán": "de", "german": "de",
	"ruso": "ru", "russian": "ru",
	"arabe": "ar", "árabe": "ar", "arabic": "ar",
	"hindi":   "hi",
	"japones": "ja", "japonés": "ja", "japanese": "ja", "jp": "ja",
	"coreano": "ko", "korean": "ko", "kr": "ko",
	"chino": "zh", "chinese": "zh", "mandarin": "zh", "mandarín": "zh", "cn": "zh",
	"neerlandes": "nl", "neerlandés": "nl", "holandes": "nl", "holandés": "nl", "dutch": "nl",
	"turco": "tr", "turkish": "tr",
	"polaco": "pl", "polish": "pl",
	"indonesio": "id", "indonesian": "id",
	"vietnamita": "vi",
	"tailandes":  "th", "tailandés": "th",
	"ucraniano": "uk",
	"rumano":    "ro",
	"griego":    "el", "greek": "el",
	"hebreo":   "he",
	"sueco":    "sv",
	"filipino": "fil", "tagalo": "fil",
	"catalan": "ca", "catalán": "ca",
	"euskera": "eu", "vasco": "eu",
	"gallego": "gl",
	"latin":   "la", "latín": "la",
}

// sinTildes quita los acentos de lo habitual en nuestros idiomas (el JS usaba
// NFD; aquí basta una tabla: no hay que traerse unicode/norm para esto).
var sinTildes = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u",
	"â", "a", "ê", "e", "î", "i", "ô", "o", "û", "u",
	"ã", "a", "õ", "o", "ç", "c", "ä", "a", "ö", "o",
)

// ResolverIdioma acepta «en», «inglés», «ingles», «english», «pt-BR»… y
// devuelve el código, o "" si no es un idioma conocido.
func ResolverIdioma(entrada string) string {
	crudo := strings.ToLower(strings.TrimSpace(entrada))
	if crudo == "" {
		return ""
	}
	base := crudo
	if i := strings.IndexAny(crudo, "-_"); i >= 0 {
		base = crudo[:i]
	}
	if _, ok := idiomas[crudo]; ok {
		return crudo
	}
	if _, ok := idiomas[base]; ok {
		return base
	}
	if c, ok := aliasIdioma[crudo]; ok {
		return c
	}
	if c, ok := aliasIdioma[base]; ok {
		return c
	}
	limpio := sinTildes.Replace(crudo)
	if c, ok := aliasIdioma[limpio]; ok {
		return c
	}
	for alias, codigo := range aliasIdioma {
		if sinTildes.Replace(alias) == limpio {
			return codigo
		}
	}
	return ""
}

// NombreIdioma da el nombre en español (o el código tal cual si es raro).
func NombreIdioma(codigo string) string {
	c := strings.ToLower(strings.TrimSpace(codigo))
	if i := strings.IndexAny(c, "-_"); i >= 0 {
		c = c[:i]
	}
	if par, ok := idiomas[c]; ok {
		return par[0]
	}
	if codigo != "" {
		return c
	}
	return "desconocido"
}

// BanderaIdioma da la banderita del código (🌐 si no se conoce).
func BanderaIdioma(codigo string) string {
	c := strings.ToLower(strings.TrimSpace(codigo))
	if i := strings.IndexAny(c, "-_"); i >= 0 {
		c = c[:i]
	}
	if par, ok := idiomas[c]; ok {
		return par[1]
	}
	return "🌐"
}

// Peticion es lo que pidió el usuario ya descifrado.
type Peticion struct {
	OK        bool
	Motivo    string // "sin-texto" cuando no hay nada que traducir
	Idioma    string
	Texto     string
	Recortado bool
}

// ParsePeticion entiende:
//
//	.tr inglés hola    → {Idioma:"en", Texto:"hola"}
//	.tr hola           → {Idioma:"es", Texto:"hola"}     (sin idioma → al del bot)
//	.tr en  (citando)  → {Idioma:"en", Texto:<lo citado>}
//	.tr     (citando)  → {Idioma:"es", Texto:<lo citado>}
func ParsePeticion(args []string, citado, porDefecto string) Peticion {
	var lista []string
	for _, a := range args {
		if strings.TrimSpace(a) != "" {
			lista = append(lista, a)
		}
	}
	citado = strings.TrimSpace(citado)

	idioma := porDefecto
	resto := lista
	if len(lista) > 0 {
		if posible := ResolverIdioma(lista[0]); posible != "" && (len(lista) > 1 || citado != "") {
			idioma = posible
			resto = lista[1:]
		}
	}

	texto := strings.TrimSpace(strings.Join(resto, " "))
	if texto == "" {
		texto = citado
	}
	if texto == "" {
		return Peticion{Motivo: "sin-texto", Idioma: idioma}
	}
	if len(texto) > TraducirMaxTexto {
		return Peticion{OK: true, Idioma: idioma, Texto: texto[:TraducirMaxTexto], Recortado: true}
	}
	return Peticion{OK: true, Idioma: idioma, Texto: texto}
}

// ── Detector de idioma propio ───────────────────────────────────────────────

// escrituras: alfabetos que cantan solos. El chino va al final: el japonés
// también usa kanji.
var escrituras = []struct {
	patron *regexp.Regexp
	codigo string
}{
	{regexp.MustCompile(`[\x{3040}-\x{309F}\x{30A0}-\x{30FF}]`), "ja"},
	{regexp.MustCompile(`[\x{AC00}-\x{D7AF}]`), "ko"},
	{regexp.MustCompile(`[\x{0400}-\x{04FF}]`), "ru"},
	{regexp.MustCompile(`[\x{0600}-\x{06FF}]`), "ar"},
	{regexp.MustCompile(`[\x{0590}-\x{05FF}]`), "he"},
	{regexp.MustCompile(`[\x{0370}-\x{03FF}]`), "el"},
	{regexp.MustCompile(`[\x{0900}-\x{097F}]`), "hi"},
	{regexp.MustCompile(`[\x{0E00}-\x{0E7F}]`), "th"},
	{regexp.MustCompile(`[\x{4E00}-\x{9FFF}]`), "zh"},
}

// vacias: las palabras que más se repiten en cada idioma. El orden de esta
// lista fija el desempate: es el orden en que se recorren los puntos.
var vacias = []struct {
	codigo   string
	palabras []string
}{
	{"es", []string{"que", "de", "no", "la", "el", "en", "y", "es", "por", "para", "con", "los", "las", "un", "una", "me", "te", "se", "mi", "tu", "pero", "como", "muy", "esta", "este", "hola", "gracias", "porque", "cuando", "donde", "tambien", "bien", "todo", "todos", "todas", "hay", "ya", "si", "buenas", "buenos", "dias", "noches", "gente", "alguien", "nadie", "ahora", "aqui", "eso", "asi", "favor", "perdon", "claro", "vale", "oye", "manana", "hoy", "ver", "hacer", "quiero", "puedo"}},
	{"en", []string{"the", "is", "and", "to", "of", "in", "you", "that", "it", "for", "on", "are", "with", "this", "have", "from", "not", "but", "what", "your", "just", "can", "will", "how", "they", "about", "there", "would", "like", "when", "thanks", "hello", "good", "morning", "night", "evening", "please", "sorry", "hey", "yes", "guys", "know", "think", "need", "really", "love", "time", "today", "tomorrow", "see", "get", "make", "want"}},
	{"pt", []string{"que", "nao", "de", "para", "com", "uma", "por", "mais", "como", "voce", "isso", "esta", "muito", "tem", "obrigado", "obrigada", "ola", "bom", "boa", "dia", "noite", "sim", "tambem", "quando", "onde", "porque", "fazer", "tudo", "bem", "favor", "gente", "agora", "hoje", "amanha", "quero"}},
	{"fr", []string{"le", "la", "les", "de", "des", "est", "une", "que", "pas", "pour", "dans", "vous", "avec", "sur", "plus", "mais", "nous", "comme", "tout", "bien", "merci", "bonjour", "bonsoir", "aussi", "quand", "salut", "oui", "non", "aujourd", "demain", "faire", "veux"}},
	{"it", []string{"che", "non", "di", "per", "una", "con", "sono", "questo", "come", "anche", "piu", "molto", "grazie", "ciao", "bene", "buongiorno", "buonasera", "quando", "perche", "tutto", "sempre", "fare", "oggi", "domani", "voglio", "si"}},
	{"de", []string{"der", "die", "das", "und", "ist", "nicht", "ein", "eine", "mit", "auf", "fur", "auch", "aber", "wie", "was", "wir", "ich", "sie", "danke", "hallo", "sehr", "wenn", "geht", "dir", "mir", "guten", "morgen", "abend", "bitte", "heute", "noch", "schon", "haben", "sein", "du", "zu", "von", "im", "nur", "mehr", "kann", "gut"}},
	{"nl", []string{"het", "een", "van", "niet", "is", "dat", "met", "voor", "maar", "ook", "naar", "zijn", "hebben", "deze", "dank", "hallo"}},
	{"ca", []string{"amb", "aixo", "molt", "tambe", "perque", "aquest", "gracies", "sempre", "nosaltres", "aquesta", "pero", "aviat"}},
}

// pistas: tildes y signos que casi solo usa un idioma (regex, código, peso).
var pistas = []struct {
	patron *regexp.Regexp
	codigo string
	peso   float64
}{
	{regexp.MustCompile(`(?i)[ñ¿¡]`), "es", 2.2},
	{regexp.MustCompile(`(?i)[ãõç]`), "pt", 2.0},
	{regexp.MustCompile(`(?i)[ßäöü]`), "de", 1.6},
	{regexp.MustCompile(`(?i)[àèìòù]`), "it", 1.2},
	{regexp.MustCompile(`(?i)[œêôûëï]`), "fr", 1.4},
	{regexp.MustCompile(`(?i)l·l|·`), "ca", 1.2},
}

// prior: empujón mínimo a los más probables en un grupo hispano; deshace
// empates tontos (un «hola» es español mucho más a menudo que catalán).
var prior = map[string]float64{"es": 0.03, "en": 0.03, "pt": 0.02, "fr": 0.01, "it": 0.01, "de": 0.01}

var soloLetras = regexp.MustCompile(`[^\p{L}\s]+`)

// DetectarIdioma adivina el idioma de un texto. Devuelve ("", 0) si no se
// atreve. Mismo algoritmo y mismos umbrales que el JS.
func DetectarIdioma(texto string) (string, float64) {
	limpio := strings.TrimSpace(texto)
	if len([]rune(limpio)) < 3 {
		return "", 0
	}
	for _, e := range escrituras {
		if e.patron.MatchString(limpio) {
			return e.codigo, 1
		}
	}

	plano := soloLetras.ReplaceAllString(sinTildes.Replace(strings.ToLower(limpio)), " ")
	palabras := strings.Fields(plano)
	if len(palabras) == 0 {
		return "", 0
	}

	puntos := map[string]float64{}
	for _, v := range vacias {
		juego := map[string]bool{}
		for _, p := range v.palabras {
			juego[p] = true
		}
		aciertos := 0
		for _, p := range palabras {
			if juego[p] {
				aciertos++
			}
		}
		puntos[v.codigo] = float64(aciertos) / float64(len(palabras))
		if aciertos > 0 {
			puntos[v.codigo] += prior[v.codigo]
		}
	}
	for _, p := range pistas {
		if p.patron.MatchString(limpio) {
			puntos[p.codigo] += 0.12 * p.peso
		}
	}

	// Orden con desempate determinista (el mapa de Go no tiene orden).
	type par struct {
		codigo string
		valor  float64
	}
	var orden []par
	for codigo, valor := range puntos {
		orden = append(orden, par{codigo, valor})
	}
	sort.Slice(orden, func(i, j int) bool {
		if orden[i].valor == orden[j].valor {
			return orden[i].codigo < orden[j].codigo
		}
		return orden[i].valor > orden[j].valor
	})
	mejor, valor := orden[0].codigo, orden[0].valor
	segundo := 0.0
	if len(orden) > 1 {
		segundo = orden[1].valor
	}
	if valor < 0.1 || valor-segundo < 0.012 {
		return "", valor
	}
	confianza := valor * 2.5
	if confianza > 1 {
		confianza = 1
	}
	return mejor, confianza
}

// ── Proveedores ─────────────────────────────────────────────────────────────
// Dos, para que una caída no deje el comando muerto. El parseo va en funciones
// sueltas para probarlo con respuestas de mentira.

// URLs como variables: las pruebas las apuntan a un httptest.Server.
var (
	urlGoogleTraductor = "https://translate.googleapis.com/translate_a/single"
	urlMyMemory        = "https://api.mymemory.translated.net/get"
)

// parseGoogle interpreta [[["hola","hello",…],…],null,"en",…].
func parseGoogle(datos []any) (texto, de string) {
	if len(datos) == 0 {
		return "", ""
	}
	trozos, ok := datos[0].([]any)
	if !ok {
		return "", ""
	}
	var b strings.Builder
	for _, t := range trozos {
		if par, ok := t.([]any); ok && len(par) > 0 {
			if s, ok := par[0].(string); ok {
				b.WriteString(s)
			}
		}
	}
	texto = strings.TrimSpace(b.String())
	if texto == "" {
		return "", ""
	}
	if len(datos) > 2 {
		if s, ok := datos[2].(string); ok {
			de = strings.SplitN(s, "-", 2)[0]
		}
	}
	return texto, de
}

// parseMyMemory interpreta {responseData:{translatedText}, responseStatus}.
func parseMyMemory(datos map[string]any) string {
	if estado, ok := datos["responseStatus"].(float64); ok && estado >= 400 {
		return ""
	}
	rd, ok := datos["responseData"].(map[string]any)
	if !ok {
		return ""
	}
	texto, _ := rd["translatedText"].(string)
	return strings.TrimSpace(texto)
}

// ResultadoTraduccion es lo que sale de Traducir (o por qué no salió).
type ResultadoTraduccion struct {
	OK        bool
	Texto     string
	De        string // idioma detectado (vacío si ni Google ni el adivino lo saben)
	A         string
	Proveedor string
	Cacheado  bool
	Motivo    string // "idioma-desconocido" · "sin-texto" · "tiempo-agotado" · "http-429"…
}

// Caché FIFO de traducciones: máximo traducirMaxCache entradas.
var (
	cacheMu    sync.Mutex
	cache      = map[string]ResultadoTraduccion{}
	cacheOrden []string
)

func cacheBuscar(clave string) (ResultadoTraduccion, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	r, ok := cache[clave]
	return r, ok
}

func cacheGuardar(clave string, r ResultadoTraduccion) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if _, existe := cache[clave]; !existe {
		cacheOrden = append(cacheOrden, clave)
		if len(cacheOrden) > traducirMaxCache {
			delete(cache, cacheOrden[0])
			cacheOrden = cacheOrden[1:]
		}
	}
	cache[clave] = r
}

// LimpiarCacheTraduccion vacía la caché (la usan las pruebas y `.cache`).
func LimpiarCacheTraduccion() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache = map[string]ResultadoTraduccion{}
	cacheOrden = nil
}

// Traducir prueba los proveedores en orden y nunca devuelve error plano:
// Motivo explica el fallo para que el comando decida qué decir.
func Traducir(ctx context.Context, texto, destino string) ResultadoTraduccion {
	limpio := strings.TrimSpace(texto)
	if len(limpio) > TraducirMaxTexto {
		limpio = limpio[:TraducirMaxTexto]
	}
	idioma := ResolverIdioma(destino)
	if limpio == "" {
		return ResultadoTraduccion{Motivo: "sin-texto"}
	}
	if idioma == "" {
		return ResultadoTraduccion{Motivo: "idioma-desconocido"}
	}

	clave := idioma + "\x00" + limpio
	if r, ok := cacheBuscar(clave); ok {
		r.Cacheado = true
		return r
	}

	proveedores := []struct {
		nombre string
		url    string
	}{
		{"Google Traductor", urlGoogleTraductor + "?client=gtx&sl=auto&tl=" +
			url.QueryEscape(idioma) + "&dt=t&q=" + url.QueryEscape(limpio)},
		{"MyMemory", urlMyMemory + "?q=" + url.QueryEscape(limpio) +
			"&langpair=" + url.QueryEscape("autodetect|"+idioma)},
	}

	ultimoFallo := ""
	for _, p := range proveedores {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
		if err != nil {
			return ResultadoTraduccion{Motivo: "sin-red"}
		}
		resp, err := pedir(req)
		if err != nil {
			if ctx.Err() != nil {
				ultimoFallo = "tiempo-agotado"
			} else {
				ultimoFallo = "sin-red"
			}
			continue
		}
		cuerpo, pasado, err := leerTecho(resp.Body, 1<<20)
		_ = resp.Body.Close()
		if err != nil || pasado {
			ultimoFallo = "respuesta-rara"
			continue
		}
		if resp.StatusCode != http.StatusOK {
			ultimoFallo = fmt.Sprintf("http-%d", resp.StatusCode)
			continue
		}

		var textoT, de string
		if p.nombre == "Google Traductor" {
			var datos []any
			if json.Unmarshal(cuerpo, &datos) != nil {
				ultimoFallo = "respuesta-rara"
				continue
			}
			textoT, de = parseGoogle(datos)
		} else {
			var datos map[string]any
			if json.Unmarshal(cuerpo, &datos) != nil {
				ultimoFallo = "respuesta-rara"
				continue
			}
			textoT = parseMyMemory(datos)
		}
		if textoT == "" {
			ultimoFallo = "respuesta-rara"
			continue
		}

		// MyMemory no dice de dónde venía: lo adivina el detector propio.
		if de == "" {
			de, _ = DetectarIdioma(limpio)
		}
		salida := ResultadoTraduccion{OK: true, Texto: textoT, De: de, A: idioma, Proveedor: p.nombre}
		cacheGuardar(clave, salida)
		return salida
	}
	if ultimoFallo == "" {
		ultimoFallo = "sin-proveedores"
	}
	return ResultadoTraduccion{Motivo: ultimoFallo}
}
