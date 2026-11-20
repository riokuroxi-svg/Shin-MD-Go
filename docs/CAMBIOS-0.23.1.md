# Cambios 0.23.1 — el código se queda a la vista hasta conectar

## El fallo

En 0.23.0, la vinculación por número hacía esto:

1. `PairPhone` pedía a WhatsApp el código de ocho caracteres.
2. Se imprimía el código.
3. `Login` devolvía `nil` inmediatamente.
4. `main` continuaba y entraba al búfer alternativo para dibujar el panel.

El paso 4 tapaba el código antes de que el usuario pudiera copiarlo. El bot aún
no estaba vinculado: obtener las ocho letras no significa que el teléfono ya las
haya aceptado.

## La barrera correcta

La vinculación es ahora una barrera síncrona situada antes de todo lo que pueda
arrancar la TUI. Requiere dos señales distintas de `whatsmeow`:

- `PairSuccess`: el teléfono aceptó el código y las credenciales quedaron
  guardadas;
- `Connected`: terminó la reconexión posterior y el socket está autenticado.

La propia documentación de `whatsmeow` avisa que `PairSuccess` suele ir seguido
de una reconexión y que no se debe usar el cliente antes de `Connected`.
`Login` no vuelve hasta recibir **las dos**, incluso si llegan en orden inverso.
Solo después `main` construye el panel.

Mientras espera:

- el código se imprime como texto estático y seleccionable, con las letras
  separadas;
- no se ejecuta `clear`;
- no se entra al búfer alternativo;
- no hay temporizador de repintado;
- `Ctrl+C` cancela la espera normalmente;
- una caducidad, `PairError`, `ConnectFailure`, `LoggedOut` o cliente obsoleto
  sale como error comprensible en vez de dejar una espera muda.

El flujo de QR usa la misma barrera. Antes volvía con `PairSuccess`, aunque la
conexión autenticada todavía no estuviera abierta.

## Pruebas de regresión

`internal/wa/vinculacion_test.go` cubre la secuencia que fallaba:

1. entrega `PairSuccess`;
2. comprueba que la función **sigue bloqueada** (el panel aún no puede existir);
3. entrega `Connected`;
4. comprueba que solo entonces termina.

También prueba el orden inverso, la caducidad, un fallo de conexión y la
cancelación. El render del código rechaza marcos TUI, arte de bloques y
secuencias de limpiar pantalla o entrar al búfer alternativo.
