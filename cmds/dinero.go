// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// dinero.go — Formato y lectura de cantidades de dinero.
//
// En el JS cada comando hacía `¥${n.toLocaleString()} ${monedas}` a mano, y
// había sitios donde se les olvidaba (o ponía ¥ duro aunque la moneda fuera
// otra). Aquí hay una sola pareja de funciones y todos los comandos pasan por
// ellas: si el dueño cambia `.setcurrency Pesos`, el símbolo cambia en todo.
package cmds

import (
	"strconv"
	"strings"

	"shin-go/internal/bot"
	"shin-go/internal/store"
)

// Moneda por defecto: la misma que el JS (g_settings.currency = 'Yenes').
const monedaPorDefecto = "Yenes"

// currencySymbol devuelve el símbolo que corresponde al nombre de la moneda.
// Se guarda el nombre ("Yenes", "Pesos"…) y se pinta el símbolo corto.
func currencySymbol(nombre string) string {
	switch strings.ToLower(strings.TrimSpace(nombre)) {
	case "yenes", "yen", "jpy":
		return "¥"
	case "pesos", "peso", "mxn", "ars", "cop", "clp":
		return "$"
	case "dolares", "dólares", "dolar", "dólar", "usd":
		return "$"
	case "euros", "euro", "eur":
		return "€"
	case "reales", "real", "brl":
		return "R$"
	case "soles", "sol", "pen":
		return "S/"
	case "bolivares", "bolívares", "ves":
		return "Bs."
	case "":
		return "¥"
	default:
		return "¥"
	}
}

// moneda es el nombre configurado (lo que se escribe en los mensajes).
func moneda(c *bot.Context) string {
	v, ok, err := c.DB.GetSetting(c.Ctx, store.SystemJID, "currency")
	if err == nil && ok && v != "" {
		return v
	}
	return monedaPorDefecto
}

// miles pone separador de miles (lo que hacía toLocaleString()).
func miles(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// money pinta "¥12,500 Yenes".
func money(c *bot.Context, n int64) string {
	m := moneda(c)
	return currencySymbol(m) + miles(n) + " " + m
}

// moneyShort pinta solo "¥12,500" (para tablas y rankings, como el JS).
func moneyShort(c *bot.Context, n int64) string {
	return currencySymbol(moneda(c)) + miles(n)
}

// parseAmount lee una cantidad escrita por el usuario.
//
// Acepta: "2500", "2,500", "2.500" (miles), "25k", "1.5k", "2m" y "all".
// `all` devuelve (0, true). Rechaza negativos, ceros y valores absurdos.
// El JS hacía `parseInt(args[0])`, con lo que "2500abc" colaba como 2500 y
// "abc" como NaN: aquí lo que no es un número limpio se rechaza.
func parseAmount(raw string) (amount int64, all bool, ok bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, false, false
	}
	if s == "all" || s == "todo" {
		return 0, true, true
	}

	// Quitar símbolos de moneda y espacios.
	s = strings.TrimLeft(s, "¥$€")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, false, false
	}

	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1_000, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "m")
	}

	ent, dec, okSep := splitNumber(s, mult > 1)
	if !okSep {
		return 0, false, false
	}

	valor := float64(0)
	if ent != "" {
		n, err := strconv.ParseInt(ent, 10, 64)
		if err != nil {
			return 0, false, false
		}
		valor = float64(n)
	}
	if dec != "" {
		f, err := strconv.ParseFloat("0."+dec, 64)
		if err != nil {
			return 0, false, false
		}
		valor += f
	}

	v := int64(valor * float64(mult))
	if v <= 0 || v > store.MaxMoney {
		return 0, false, false
	}
	return v, false, true
}

// splitNumber valida la parte numérica y la separa en entero y decimales.
//
// Acepta "2500", "2,500", "2.500" y "1,000,000" (separador de miles cada tres
// dígitos) y, solo cuando hay sufijo k/m, "1.5" o "1,5" como decimal.
// Todo lo demás se rechaza: "1..2", "2500abc", "1,00,0", "" …
func splitNumber(s string, decimalPermitido bool) (ent, dec string, ok bool) {
	if s == "" {
		return "", "", false
	}
	// Un solo tipo de separador: mezclar "," y "." siempre es ambiguo.
	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		return "", "", false
	}
	sep := ""
	if strings.Contains(s, ",") {
		sep = ","
	} else if strings.Contains(s, ".") {
		sep = "."
	}
	if sep == "" {
		for _, r := range s {
			if r < '0' || r > '9' {
				return "", "", false
			}
		}
		return s, "", true
	}

	partes := strings.Split(s, sep)
	for _, p := range partes {
		if p == "" {
			return "", "", false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return "", "", false
			}
		}
	}

	switch {
	case len(partes) == 2 && decimalPermitido && len(partes[1]) <= 2:
		// 1.5k → 1 entero, 5 decimales
		return partes[0], partes[1], true
	default:
		// Separador de miles: todos los grupos menos el primero son de tres
		// dígitos, y el primero no puede pasarse de tres si hay más grupos.
		for i := 1; i < len(partes); i++ {
			if len(partes[i]) != 3 {
				return "", "", false
			}
		}
		if len(partes) > 1 && (len(partes[0]) < 1 || len(partes[0]) > 3) {
			return "", "", false
		}
		return strings.Join(partes, ""), "", true
	}
}

// formatWait pinta una espera: "3 minutos 12 segundos".
func formatWait(seconds int64) string {
	if seconds <= 0 {
		return "0 segundos"
	}
	dias := seconds / 86400
	horas := (seconds % 86400) / 3600
	minutos := (seconds % 3600) / 60
	segs := seconds % 60

	partes := make([]string, 0, 3)
	add := func(n int64, uno, varios string) {
		if n <= 0 {
			return
		}
		if n == 1 {
			partes = append(partes, "1 "+uno)
		} else {
			partes = append(partes, strconv.FormatInt(n, 10)+" "+varios)
		}
	}
	add(dias, "día", "días")
	add(horas, "hora", "horas")
	add(minutos, "minuto", "minutos")
	if len(partes) == 0 {
		add(segs, "segundo", "segundos")
	}
	return strings.Join(partes, " ")
}
