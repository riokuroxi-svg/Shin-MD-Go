// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos_ayuda.go — Utilidades compartidas por los comandos de grupo.
package cmds

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/wa"
)

// groupInfo lee la información del grupo sin caché. Los comandos de admin
// tienen que decidir con el estado real: la caché de un minuto es perfecta para
// permisos de mensajes, pero no para "¿este ya es admin?" justo antes de
// ascenderlo.
func groupInfo(c *bot.Context) *types.GroupInfo {
	info, err := c.Bot.GroupInfo(c.Ctx, c.Chat)
	if err != nil {
		return nil
	}
	return info
}

// skipMention quita los argumentos que son menciones (@521…), para que la razón
// de un `.warn` no empiece con el número del advertido.
func skipMention(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "@") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// argAt devuelve el argumento n (0 si no existe).
func argAt(args []string, n int) string {
	if n < 0 || n >= len(args) {
		return ""
	}
	return args[n]
}

// fecha pinta una marca de tiempo en algo que se lea en un chat.
func fecha(ms int64) string {
	if ms <= 0 {
		return "sin fecha"
	}
	return time.UnixMilli(ms).Local().Format("02/01/2006 15:04")
}

// activitySince cuenta los mensajes de alguien desde hace `d`.
func activitySince(c *bot.Context, who types.JID, d time.Duration) (int64, error) {
	_, recientes, err := c.DB.Messages(c.Ctx, c.Chat.String(), userKey(c, who), time.Now().Add(-d))
	return recientes, err
}

// quotedImage devuelve el ImageMessage del mensaje al que se responde (nil si
// no hay imagen). Se usa en `.setgpbanner`: la foto del grupo solo cambia con
// una imagen que alguien haya citado, nunca descargando cosas por gusto.
func quotedImage(c *bot.Context) *waE2E.ImageMessage {
	msg := wa.Unwrap(c.Event.Message)
	if msg == nil {
		return nil
	}
	ci := wa.ContextInfo(msg)
	if ci == nil {
		return nil
	}
	citado := ci.GetQuotedMessage()
	if citado == nil {
		return nil
	}
	inner := wa.Unwrap(citado)
	if inner == nil {
		return nil
	}
	return inner.GetImageMessage()
}

// textoPlantilla sustituye los marcadores de la bienvenida y la despedida.
//
// Marcadores (los mismos que el JS): @user, #grupo, #nombre (alias de #grupo),
// #miembros. Se sustituyen con TOPE de longitud: una plantilla guardada con
// 5.000 caracteres no debe generar un mensaje gigante cada vez que entra
// alguien.
func textoPlantilla(plantilla, nombreGrupo string, miembros int, mencion string) string {
	out := plantilla
	out = strings.ReplaceAll(out, "@user", mencion)
	out = strings.ReplaceAll(out, "#grupo", nombreGrupo)
	out = strings.ReplaceAll(out, "#nombre", nombreGrupo)
	out = strings.ReplaceAll(out, "#miembros", fmt.Sprint(miembros))
	if len([]rune(out)) > 900 {
		out = string([]rune(out)[:900])
	}
	return out
}
