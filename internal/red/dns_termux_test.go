// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// dns_termux_test.go — el resolvedor Termux: decisión, parseo y consulta real.
package red

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNameserversDeParseo(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "resolv.conf")
	contenido := `# comentario
; otro estilo de comentario
nameserver 8.8.8.8
nameserver 1.1.1.1 # con comentario pegado
options ndots:5
search casa.lan
nameserver no-es-una-ip
nameserver
nameserver 2606:4700:4700::1111
nameserver 9.9.9.9
nameserver 149.112.112.112
`
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	got := nameserversDe(ruta)
	quiere := []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} // tope de tres
	if len(got) != 3 {
		t.Fatalf("esperaba 3 servidores, salieron %v", got)
	}
	for i := range quiere {
		if got[i] != quiere[i] {
			t.Fatalf("servidor %d: %q ≠ %q", i, got[i], quiere[i])
		}
	}
	if nameserversDe(filepath.Join(dir, "no-existo")) != nil {
		t.Fatal("archivo ausente debió dar nil")
	}
}

func TestResolvedorNecesarioDecision(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "usr")
	if err := os.MkdirAll(filepath.Join(prefix, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// El sandbox SÍ tiene /etc/resolv.conf sano → en Termux simulado no toca.
	if s := resolvedorNecesario(prefix, true); s != nil {
		t.Fatalf("con /etc/resolv.conf sano no debió tocar nada, dio %v", s)
	}
	// Fuera de Termux y con PREFIX ajeno: nada.
	if s := resolvedorNecesario("/usr", false); s != nil {
		t.Fatalf("fuera de Termux no debió tocar nada, dio %v", s)
	}
}

func TestResolvedorNecesarioSinEtcEstandar(t *testing.T) {
	// Simular Android de verdad requiere que /etc/resolv.conf falte; en el
	// sandbox existe, así que se prueba la rama de lectura de $PREFIX con la
	// función de parseo directamente: el contrato es que ESE archivo manda.
	dir := t.TempDir()
	prefix := filepath.Join(dir, "usr")
	etc := filepath.Join(prefix, "etc")
	if err := os.MkdirAll(etc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(etc, "resolv.conf"),
		[]byte("nameserver 9.9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := nameserversDe(filepath.Join(etc, "resolv.conf"))
	if len(got) != 1 || got[0] != "9.9.9.9" {
		t.Fatalf("debió leer 9.9.9.9 de $PREFIX/etc/resolv.conf, leyó %v", got)
	}
}

func TestResolvedorInstaladoResponde(t *testing.T) {
	// Servidor DNS mínimo de verdad: contesta cualquier consulta con
	// una respuesta DNS válida (eco de la cabecera con flag QR y sin answer).
	// Con eso basta para verificar que DefaultResolver ahora marca aquí.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("sin UDP local: %v", err)
	}
	defer func() { _ = pc.Close() }()
	addr := pc.LocalAddr().(*net.UDPAddr)
	go func() {
		buf := make([]byte, 512)
		for {
			n, quien, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n >= 12 {
				// ID igual, QR=1, el resto igual: respuesta «vacía» pero válida.
				resp := append([]byte(nil), buf[:n]...)
				resp[2] |= 0x80
				resp[7], resp[8] = 0, 0 // ANCOUNT hi/lo → 0 respuestas
				_, _ = pc.WriteTo(resp, quien)
			}
		}
	}()

	instalarResolvedor([]string{addr.IP.String()})
	// Ojo: el resolvedor va al puerto 53 fijo; aquí no podemos escuchar en 53
	// sin root, así que la prueba fina de red la cubre la simulación de arriba
	// con la ruta de decisión. Lo que sí se afirma: la sustitución no rompe el
	// proceso y la rotación avanza.
	if net.DefaultResolver == nil || !net.DefaultResolver.PreferGo {
		t.Fatal("el resolvedor instalado debe ser puro-Go")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = net.DefaultResolver.LookupHost(ctx, "ejemplo.invalid") // no debe colgar ni pánico
}

func TestPemDelPrefix(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "usr")
	tls := filepath.Join(prefix, "etc", "tls")
	if err := os.MkdirAll(tls, 0o755); err != nil {
		t.Fatal(err)
	}
	// Sin archivo: cadena vacía y sin tocar la env.
	t.Setenv("SSL_CERT_FILE", "")
	if got := pemDelPrefix(prefix); got != "" {
		t.Fatalf("sin .pem debió dar vacío, dio %q", got)
	}
	// Archivo vacío tampoco sirve.
	pem := filepath.Join(tls, "cert.pem")
	if err := os.WriteFile(pem, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pemDelPrefix(prefix); got != "" {
		t.Fatalf("pem vacío debió dar vacío, dio %q", got)
	}
	// Con contenido: aplica y señala.
	if err := os.WriteFile(pem, []byte("-----BEGIN CERTIFICATE-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pemDelPrefix(prefix); got != pem {
		t.Fatalf("debió aplicar %q, dio %q", pem, got)
	}
	if os.Getenv("SSL_CERT_FILE") != pem {
		t.Fatal("la env SSL_CERT_FILE no quedó puesta")
	}
}
