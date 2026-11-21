// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package cmds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	menuWeatherFallback = "🌤️ No disponible | 🌡️ --°C"
	menuWeatherCity     = "Mexico City"
)

var menuWeatherState struct {
	sync.Mutex
	city  string
	value string
	until time.Time
}

// menuWeather porta el resumen meteorológico del menú de JS. Una respuesta
// correcta dura diez minutos en caché y un fallo dura uno para no castigar el
// menú cuando el proveedor cae.
func menuWeather(ctx context.Context, city string) string {
	city = strings.TrimSpace(city)
	if city == "" {
		city = menuWeatherCity
	}

	now := time.Now()
	menuWeatherState.Lock()
	defer menuWeatherState.Unlock()
	if menuWeatherState.city == city && menuWeatherState.value != "" && now.Before(menuWeatherState.until) {
		return menuWeatherState.value
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	value, err := fetchMenuWeather(ctx, city)
	if err != nil {
		value = menuWeatherFallback
		menuWeatherState.until = now.Add(time.Minute)
	} else {
		menuWeatherState.until = now.Add(10 * time.Minute)
	}
	menuWeatherState.city = city
	menuWeatherState.value = value
	return value
}

func fetchMenuWeather(ctx context.Context, city string) (string, error) {
	geoURL := "https://geocoding-api.open-meteo.com/v1/search?" + url.Values{
		"name":  {city},
		"count": {"1"},
	}.Encode()
	var geo struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := fetchMenuJSON(ctx, geoURL, &geo); err != nil {
		return "", err
	}
	if len(geo.Results) == 0 {
		return "", errors.New("ciudad sin coordenadas")
	}

	forecastURL := "https://api.open-meteo.com/v1/forecast?" + url.Values{
		"latitude":  {strconv.FormatFloat(geo.Results[0].Latitude, 'f', -1, 64)},
		"longitude": {strconv.FormatFloat(geo.Results[0].Longitude, 'f', -1, 64)},
		"current":   {"temperature_2m,weather_code"},
	}.Encode()
	var forecast struct {
		Current *struct {
			Temperature float64 `json:"temperature_2m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
	}
	if err := fetchMenuJSON(ctx, forecastURL, &forecast); err != nil {
		return "", err
	}
	if forecast.Current == nil {
		return "", errors.New("pronóstico sin datos actuales")
	}

	return fmt.Sprintf("%s | 🌡️ %d°C",
		menuWeatherCondition(forecast.Current.WeatherCode),
		int(math.Round(forecast.Current.Temperature))), nil
}

func fetchMenuJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("clima respondió HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst)
}

func menuWeatherCondition(code int) string {
	switch code {
	case 0:
		return "☀️ Despejado"
	case 1:
		return "🌤️ Mayormente despejado"
	case 2:
		return "⛅ Parcialmente nublado"
	case 3:
		return "☁️ Nublado"
	case 45:
		return "🌫️ Niebla"
	case 48:
		return "🌫️ Niebla densa"
	case 51, 53, 55:
		return "🌦️ Llovizna"
	case 61:
		return "🌧️ Lluvia ligera"
	case 63:
		return "🌧️ Lluvia moderada"
	case 65:
		return "⛈️ Lluvia fuerte"
	case 80, 81, 82:
		return "🌦️ Chubascos"
	case 95, 96, 99:
		return "⛈️ Tormenta eléctrica"
	default:
		return "🌤️ Despejado"
	}
}
