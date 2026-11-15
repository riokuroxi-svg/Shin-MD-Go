// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// precios.go — Puerto de cmds/utils/crypto.js (CoinGecko, sin key).
// Mismos alias que el JS: btc → bitcoin, sol → solana…
package apis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// urlCoinGecko es variable para que las pruebas lo apunten a un httptest.Server.
var urlCoinGecko = "https://api.coingecko.com/api/v3/simple/price"

// CryptoPredeterminados son los que salen cuando no se pide nada concreto.
var CryptoPredeterminados = []string{"bitcoin", "ethereum", "solana"}

// aliasCrypto: como lo escribe la gente → id de CoinGecko.
var aliasCrypto = map[string]string{
	"btc": "bitcoin", "bitcoin": "bitcoin",
	"eth": "ethereum", "ethereum": "ethereum",
	"sol": "solana", "solana": "solana",
	"doge": "dogecoin", "dogecoin": "dogecoin",
	"xrp": "ripple", "ripple": "ripple",
	"ada": "cardano", "cardano": "cardano",
	"ton":  "the-open-network",
	"usdt": "tether", "tether": "tether",
}

// ResolverCrypto traduce el nombre al id de CoinGecko ("" si ni se intenta).
func ResolverCrypto(nombre string) string {
	n := strings.ToLower(strings.TrimSpace(nombre))
	if c, ok := aliasCrypto[n]; ok {
		return c
	}
	return n
}

// Precio es lo que vale una moneda en los dos mercados que miramos.
type Precio struct {
	USD float64
	MXN float64
}

// ParCrypto es una moneda con su precio, en el orden en que se pidió.
type ParCrypto struct {
	ID string
	P  Precio
}

// Precios consulta CoinGecko. La lista sale en el orden pedido (la API
// devuelve un mapa, que no tiene orden). Si ninguna de las monedas existe,
// el error es "no-encontrada".
func Precios(ctx context.Context, ids []string) ([]ParCrypto, error) {
	if len(ids) == 0 {
		ids = CryptoPredeterminados
	}
	limpio := make([]string, 0, len(ids))
	vistos := map[string]bool{}
	for _, id := range ids {
		id = ResolverCrypto(id)
		if id != "" && !vistos[id] && len(limpio) < 12 { // techo anti-muro de texto
			limpio = append(limpio, id)
			vistos[id] = true
		}
	}
	if len(limpio) == 0 {
		return nil, fmt.Errorf("no-encontrada")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		urlCoinGecko+"?ids="+strings.Join(limpio, ",")+"&vs_currencies=usd,mxn", nil)
	if err != nil {
		return nil, err
	}
	resp, err := pedir(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tiempo-agotado")
		}
		return nil, fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	cuerpo, pasado, err := leerTecho(resp.Body, 1<<20)
	if err != nil || pasado {
		return nil, fmt.Errorf("respuesta-rara")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http-%d", resp.StatusCode)
	}

	var datos map[string]map[string]float64
	if json.Unmarshal(cuerpo, &datos) != nil {
		return nil, fmt.Errorf("respuesta-rara")
	}

	var out []ParCrypto
	for _, id := range limpio {
		if p, ok := datos[id]; ok {
			out = append(out, ParCrypto{ID: id, P: Precio{USD: p["usd"], MXN: p["mxn"]}})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no-encontrada")
	}
	return out, nil
}
