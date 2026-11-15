// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// buscarbin.go — localizar ejecutables sin pasar por faccessat2.
package utilidades

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoBinario dice que el ejecutable pedido no está (o no tiene bit x).
var ErrNoBinario = errors.New("el binario no está en el PATH o no es ejecutable")

// BuscarBin resuelve un nombre de ejecutable a su ruta recorriendo el PATH
// a mano, con os.Stat.
//
// ¿Por qué no os/exec.LookPath? LookPath llama a Eaccess y en linux/arm64
// Eaccess baja a la syscall faccessat2. Varios kernels de Android la vetan
// por seccomp y la respuesta es SIGSYS: el proceso muere en el acto, sin
// error que se pueda capturar. os.Stat usa newfstatat, que esos filtros sí
// dejan pasar. Y de regalo: si después se le pasa a exec.Command la ruta
// CON separador, Go no repite la búsqueda por dentro.
func BuscarBin(nombre string) (string, error) {
	if nombre == "" {
		return "", ErrNoBinario
	}
	if strings.ContainsAny(nombre, `/\`) {
		// Ya es una ruta: vale si existe y tiene bit de ejecución.
		if esEjecutable(nombre) {
			return nombre, nil
		}
		return "", fmt.Errorf("%w: %s", ErrNoBinario, nombre)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" { // PATH con «::» quiere decir el directorio actual
			dir = "."
		}
		camino := filepath.Join(dir, nombre)
		if esEjecutable(camino) {
			return camino, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrNoBinario, nombre)
}

// HayBin es el «¿existe?» plano: sustituye a `exec.LookPath(n) == nil`.
func HayBin(nombre string) bool {
	_, err := BuscarBin(nombre)
	return err == nil
}

func esEjecutable(camino string) bool {
	st, err := os.Stat(camino)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	// Ejecutable si CUALQUIERA (dueño/grupo/otros) tiene su bit x; el bit por
	// usuario exacto lo complicaría y aquí la diferencia no compra nada.
	return st.Mode().Perm()&0o111 != 0
}
