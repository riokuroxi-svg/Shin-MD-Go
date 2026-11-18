// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// owner_extra.go — el «exec» del owner del JS (.ex / .e): ejecutar shell en
// el servidor con candados: solo owner, plazo de 30 s, salida recortada, y
// NUNCA el panel ni el STDIO del bot (el JS también lo aisló: es child, no
// hijo suicida). Errores de lectura salen tal cual, recortados: son del dueño.
package cmds

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/media"
	"shin-go/internal/utilidades"
	"shin-go/internal/wa"
)

func init() {
	bot.Register(bot.Command{
		Name: "ex", Aliases: []string{"e"}, Category: bot.CatOwner, OwnerOnly: true,
		Desc: "Ejecuta un comando de shell (owner; 30 s de plazo, salida recortada).",
		Run:  cmdExec,
	})
	bot.Register(bot.Command{
		Name: "audioeffect", Aliases: []string{"bass", "nightcore", "slow", "robot", "fast", "squirrel", "reverse", "blown"},
		Category: bot.CatUtils,
		Desc:     "Efecto de audio sobre la nota/fichero citado (necesita ffmpeg): .nightcore, .bass, .slow, .robot.",
		Run:      cmdAudioFx,
	})
}

// cmdExec ejecuta y devuelve stdout+stderr. El plazo lo hace contexto y la
// salida se recorta a 3.500 runas (como el resto del bot).
func cmdExec(c *bot.Context) error {
	orden := strings.TrimSpace(c.Text)
	if orden == "" {
		return c.Reply("*.ex <comando>* — corre en el servidor con plazo de 30 s.")
	}
	ctx, cancel := context.WithTimeout(c.Ctx, 30*time.Second)
	defer cancel()

	shell := shellDisponible()
	if shell == "" {
		return c.Reply("《✧》 No encuentro ni `sh` ni `bash` en el PATH de este aparato.")
	}
	// Sin auto-prueba previa: la llamada real de abajo es la prueba; una
	// ejecución extra en cada .ex solo inflaría la espera.
	cmd := exec.CommandContext(ctx, shell, "-c", orden)
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	salida, err := cmd.CombinedOutput()

	cabe := strings.TrimSpace(string(salida))
	if cabe == "" {
		cabe = "(sin salida)"
	}
	if len([]rune(cabe)) > 3400 {
		cabe = string([]rune(cabe)[:3400]) + "\n…(cortado)"
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return c.Reply("```sh\n$ " + orden + "\n\n(se pasó de los 30 s; lo maté)\n```")
		}
		return c.Reply("```sh\n$ " + orden + "\n\n" + cabe + "\n\n(exit " + cerca(err.Error(), 60) + ")\n```")
	}
	return c.Reply("```sh\n$ " + orden + "\n\n" + cabe + "\n```")
}

// shellDisponible prefiere bash y si no, sh; devuelve la ruta ABSOLUTA o "".
// Nada de "/bin/sh" a ciegas (en Termux /bin/sh no existe) y nada de
// exec.LookPath (en varios Android muere por SIGSYS): todo por BuscarBin.
func shellDisponible() string {
	if p, err := utilidades.BuscarBin("bash"); err == nil {
		return p
	}
	p, _ := utilidades.BuscarBin("sh")
	return p
}

// cmdAudioFx: el JS pasaba la nota por ffmpeg con un filtro; aquí igual, con
// los mismos nombres de filtro que el JS (asetrate para nightcore, bass, etc).
func cmdAudioFx(c *bot.Context) error {
	filtro := map[string]string{
		"bass":        "bass=g=12",
		"nightcore":   "asetrate=44100*1.25,aresample=44100",
		"slow":        "asetrate=44100*0.75,aresample=44100",
		"robot":       "afftfilt=real='hypot(re,im)*sin(0)':imag='hypot(re,im)*cos(0)':win_size=512:overlap=0.75",
		"fast":        "asetrate=44100*1.5,aresample=44100",
		"squirrel":    "asetrate=44100*1.8,aresample=44100",
		"audioeffect": "",
	}[c.Command]
	if filtro == "" && c.Command == "audioeffect" {
		return c.Reply("Efectos: *.bass* · *.nightcore* (rápido fino) · *.slow* · *.robot* · *.fast* · *.squirrel*\n(responde al audio con el comando)")
	}
	if !media.HayFFmpeg() {
		return c.Reply("Este efecto necesita ffmpeg instalado en el servidor (el brat también; sin él no hay audio).")
	}
	// responder a audio
	medio, ok := wa.MedioDeEntrada(c.Event)
	if !ok || (medio.Tipo != wa.MedioAudio) {
		return c.Reply("responde a un *audio* o a una nota de voz con *." + c.Command + "*.")
	}
	datos, err := c.Bot.DownloadMedia(c.Ctx, medio)
	if err != nil || len(datos) == 0 {
		return c.Reply("no pude bajar el audio.")
	}
	salida, err := media.EfectoAudio(c.Ctx, datos, filtro)
	if err != nil {
		return c.Reply("ffmpeg no pudo con ese audio (" + cerca(err.Error(), 80) + ").")
	}
	return c.Bot.SendAudioVoz(c.Ctx, c.Chat, salida, c.Priority())
}
