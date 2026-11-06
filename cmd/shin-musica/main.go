// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// shin-musica — herramienta de laboratorio para el bloque de música.
//
// Sirve para comprobar contra las APIs de verdad, sin arrancar WhatsApp:
//
//	go run ./cmd/shin-musica buscar "lofi"
//	go run ./cmd/shin-musica bajar  "lofi" -o /tmp/cancion.mp3
//	go run ./cmd/shin-musica ficha  "daft punk"
//
// No forma parte del bot: es el banco de pruebas de internal/musica.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shin-go/internal/musica"
)

func main() {
	if len(os.Args) < 2 {
		uso()
	}
	orden := os.Args[1]

	// Los flags se separan a mano: la consulta puede llevar espacios y también
	// pueden venir flags detrás ("bajar lofi beats -n 3 -o x.mp3").
	salida := ""
	maximo := 5
	var palabras []string
	resto := os.Args[2:]
	for i := 0; i < len(resto); i++ {
		switch resto[i] {
		case "-o", "--o":
			if i+1 < len(resto) {
				salida = resto[i+1]
				i++
			}
		case "-n", "--n":
			if i+1 < len(resto) {
				if v, err := strconv.Atoi(resto[i+1]); err == nil && v > 0 && v <= 25 {
					maximo = v
				}
				i++
			}
		default:
			palabras = append(palabras, resto[i])
		}
	}
	if len(palabras) == 0 {
		uso()
	}
	consulta := strings.Join(palabras, " ")

	c := musica.Nuevo(musica.Opciones{Timeout: 20 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch orden {
	case "buscar", "ficha":
		buscar(ctx, c, consulta, maximo)

	case "bajar":
		bajar(ctx, c, consulta, maximo, salida)

	default:
		uso()
	}
}

func buscar(ctx context.Context, c *musica.Cliente, consulta string, n int) {
	inicio := time.Now()
	res := c.Buscar(ctx, consulta, n)
	fmt.Printf("consulta: %q · %d resultados en %s\n\n",
		consulta, len(res.Pistas), time.Since(inicio).Round(time.Millisecond))

	imprimir(res.Pistas)

	if len(res.Fallos) > 0 {
		fmt.Println("\nfuentes con problemas:")
		for f, e := range res.Fallos {
			fmt.Printf("  %-8s %s\n", f, e)
		}
	}
}

func bajar(ctx context.Context, c *musica.Cliente, consulta string, n int, destino string) {
	res := c.Buscar(ctx, consulta, n)
	if len(res.Pistas) == 0 {
		fatal("sin resultados para %q", consulta)
	}

	// Se prueban las candidatas libres en orden: si una pesa más que el tope, se
	// pasa a la siguiente en vez de rendirse. Un mix de tres horas es un caso
	// real (y frecuente) en Audius.
	if destino == "" {
		destino = filepath.Join(os.TempDir(), "shin-audio.mp3")
	}

	var (
		elegida *musica.Pista
		fallos  []string
	)
	intentos := 0
	for i := range res.Pistas {
		p := &res.Pistas[i]
		if !p.Libre || intentos >= 4 {
			continue
		}
		intentos++

		archivo, err := os.Create(destino)
		if err != nil {
			fatal("no se pudo crear %s: %v", destino, err)
		}
		fmt.Printf("bajando: %s — %s (%s) · fuente %s\n", p.Titulo, p.Artista, p.Minutos(), p.Fuente)

		inicio := time.Now()
		n, err := c.Descargar(ctx, *p, archivo)
		archivo.Close()
		if err != nil {
			os.Remove(destino)
			fallos = append(fallos, fmt.Sprintf("%s: %v", recorta(p.Titulo, 40), err))
			fmt.Printf("  descartada: %v\n", err)
			continue
		}

		elegida = p
		fmt.Printf("  %d bytes (%.2f MB) en %s · %.0f KB/s\n",
			n, float64(n)/(1<<20), time.Since(inicio).Round(time.Millisecond),
			float64(n)/1024/time.Since(inicio).Seconds())
		break
	}

	if elegida == nil {
		fmt.Println("\nno se pudo bajar ninguna candidata:")
		for _, f := range fallos {
			fmt.Println("  -", f)
		}
		var soloPreview []musica.Pista
		for _, p := range res.Pistas {
			if p.Preview != "" {
				soloPreview = append(soloPreview, p)
			}
		}
		if len(soloPreview) > 0 {
			fmt.Println("\nsolo quedan previews de 30 s:")
			imprimir(soloPreview)
		}
		os.Exit(2)
	}

	fmt.Printf("guardado: %s\n", destino)
	fmt.Printf("  cabecera: %s\n", leerCabecera(destino))
}

func imprimir(pistas []musica.Pista) {
	fmt.Printf("%-3s %-8s %-6s %-1s %-38s %-24s %s\n", "#", "fuente", "dur", "", "título", "artista", "audio")
	fmt.Println(strings.Repeat("-", 100))
	for i, p := range pistas {
		audio := "solo preview"
		if p.Libre {
			audio = "completo ✓"
		}
		titulo := recorta(p.Titulo, 38)
		artista := recorta(p.Artista, 24)
		fmt.Printf("%-3d %-8s %-6s %-1s %-38s %-24s %s\n", i+1, p.Fuente, p.Minutos(), "", titulo, artista, audio)
	}
}

// leerCabecera dice qué formato llegó, sin ffmpeg: basta mirar los primeros
// bytes. Es la comprobación honesta de que lo descargado es audio y no un HTML
// de error disfrazado.
func leerCabecera(ruta string) string {
	f, err := os.Open(ruta)
	if err != nil {
		return "no se pudo leer"
	}
	defer f.Close()

	buf := make([]byte, 12)
	n, _ := f.Read(buf)
	buf = buf[:n]

	switch {
	case len(buf) >= 3 && string(buf[:3]) == "ID3":
		return "MP3 con etiqueta ID3 ✓"
	case len(buf) >= 4 && string(buf[:4]) == "fLaC":
		return "FLAC ✓"
	case len(buf) >= 4 && string(buf[:4]) == "OggS":
		return "OGG/Opus ✓"
	case len(buf) >= 8 && string(buf[4:8]) == "ftyp":
		return "MP4/M4A ✓"
	case len(buf) >= 4 && buf[0] == 0xFF && buf[1]&0xE0 == 0xE0:
		return "MP3 sin etiqueta ✓"
	case len(buf) >= 5 && string(buf[:5]) == "<!DOC":
		return "¡HTML! no es audio"
	default:
		return fmt.Sprintf("desconocida (% x)", buf)
	}
}

func sanea(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	salida := strings.Trim(b.String(), "-")
	if salida == "" {
		salida = "audio"
	}
	if len(salida) > 40 {
		salida = salida[:40]
	}
	return salida
}

func recorta(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func fatal(formato string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+formato+"\n", args...)
	os.Exit(1)
}

func uso() {
	fmt.Fprintln(os.Stderr, `uso: shin-musica <buscar|bajar|ficha> "consulta" [-n 5] [-o archivo.mp3]`)
	os.Exit(1)
}
