// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// vinculacion.go — la conversación de arranque: quién entra y cómo.
//
// La regla de siempre se respeta tal cual (ver internal/wa/vinculacion.go):
//
//	· número escrito a mano (--code 521… / PAIRING_NUMBER) → se usa y ya;
//	· sesión registrada → se conecta y no se pregunta nada;
//	· sin terminal → se explica la orden exacta y se sale;
//	· con terminal → se pregunta.
//
// Lo nuevo es CÓMO se pregunta: un menú de dos opciones (código de 8 letras
// o QR), con la recomendación marcada y el número escrito de forma natural
// («+52 …»), reintentando si el dedazo lo hace ilegible.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"shin-go/internal/consola"
	"shin-go/internal/wa"
	"shin-go/internal/xlog"
)

// vincular deja el cliente conectado (o guardado) y devuelve el primer error
// de verdad. Un Ctrl+C a mitad no es un error: se devuelve ctx.Err() y quien
// llama lo trata como «cancelado».
func vincular(ctx context.Context, client *wa.Client, log *xlog.Logger) error {
	// Un número escrito a mano manda siempre: ni menú ni preguntas.
	if numero := numeroDeBandera(); numero != "" {
		log.Info("vinculando con el número " + numero + ".")
		return client.Login(ctx, numero)
	}

	if client.LoggedIn() {
		consola.Bien("sesión guardada: se conecta sin volver a vincular")
		return client.Login(ctx, "")
	}

	if !wa.HayTerminal() {
		wa.ExplicarSinTerminal(os.Stderr, "no hay sesión")
		return errors.New("sin sesión y sin terminal: hace falta --code 521… o PAIRING_NUMBER=521…")
	}

	// 1) Cómo entrar.
	metodo, err := elegirMetodo()
	if err != nil {
		return err
	}

	// 2) El QR no necesita número; el código sí (es la cuenta que lo pide).
	if metodo == wa.VinculacionQR {
		avisoQR()
		return client.LoginPorQR(ctx)
	}

	wa.PanelVinculacion(os.Stdout)
	numero := wa.PreguntarNumero(os.Stdout)
	if numero == "" {
		return errors.New("no se pudo leer un número de WhatsApp")
	}
	log.Info("vinculando con el número " + numero + ".")
	return client.Login(ctx, numero)
}

// numeroDeBandera: --code <número> o PAIRING_NUMBER, ya normalizado ("" si
// no hay ninguno). Es el camino de los servicios y del arranque automático.
func numeroDeBandera() string {
	if numero := wa.NormalizarTelefono(numeroDeCodigo); numero != "" {
		return numero
	}
	return wa.NormalizarTelefono(os.Getenv("PAIRING_NUMBER"))
}

// elegirMetodo pregunta cómo se quiere vincular. El valor por defecto es el
// código de 8 letras porque es el único que funciona en el propio móvil
// donde corre el bot (el QR necesita otra cámara).
func elegirMetodo() (wa.MetodoDeVinculacion, error) {
	opciones := []consola.Opcion{
		{
			Tecla:   "1",
			Titulo:  "Código de 8 letras  (recomendado)",
			Detalle: "te lo da este bot y lo escribes en WhatsApp",
		},
		{
			Tecla:   "2",
			Titulo:  "Código QR",
			Detalle: "para escanear desde OTRO teléfono (útil en un VPS)",
		},
	}
	tecla, err := consola.Elegir(os.Stdout, "¿Cómo quieres vincular el bot?", opciones)
	if err != nil {
		return 0, err
	}
	if tecla == "2" {
		return wa.VinculacionQR, nil
	}
	return wa.VinculacionCodigo, nil
}

// avisoQR: si el bot corre en el propio móvil, el QR no sirve. Se avisa UNA
// vez, con la salida a mano, y se sigue: nunca se bloquea al usuario.
func avisoQR() {
	fmt.Fprintln(os.Stdout)
	if esTermux() {
		consola.Ojo("corres el bot en el propio teléfono: ahí no hay cámara que escanee este QR.")
		consola.Frase(consola.Tenue("si te equivocaste, reinicia y elige 1) Código de 8 letras."))
		fmt.Fprintln(os.Stdout)
		return
	}
	consola.Frase(consola.Tenue("escanéalo con WhatsApp del teléfono que llevará la cuenta."))
	fmt.Fprintln(os.Stdout)
}

func esTermux() bool {
	return os.Getenv("TERMUX_VERSION") != "" || contiene(os.Getenv("PREFIX"), "com.termux")
}

func contiene(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// paqueteDeComandos: lo que enseña la tecla `p` — cómo se escriben los
// comandos y dónde mirar la lista completa, sin soltar un manual de 20
// pantallas en mitad del arranque.
func paqueteDeComandos(unicos int, prefijos []string) {
	fmt.Fprintln(os.Stdout)
	consola.Caja(os.Stdout, "comandos",
		consola.Papel(fmt.Sprintf("%d comandos cargados · prefijos [%s]", unicos, join(prefijos, " "))),
		"",
		consola.Papel("en WhatsApp:")+" "+consola.Tenue(".menu")+"  la lista entera",
		consola.Papel("              ")+consola.Tenue(".ping")+"  ¿está vivo?",
		consola.Papel("              ")+consola.Tenue(".info")+"  versión y datos",
		"",
		consola.Tenue("los prefijos se cambian con BOT_PREFIX en el .env"),
	)
	fmt.Fprintln(os.Stdout)
}

func join(xs []string, sep string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += sep
		}
		out += x
	}
	return out
}
