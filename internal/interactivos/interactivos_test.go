// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package interactivos

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func jidPrivado(t *testing.T) types.JID {
	t.Helper()
	jid, err := types.ParseJID("5215555555555@s.whatsapp.net")
	if err != nil {
		t.Fatal(err)
	}
	return jid
}

func jidGrupo(t *testing.T) types.JID {
	t.Helper()
	jid, err := types.ParseJID("1234-5678@g.us")
	if err != nil {
		t.Fatal(err)
	}
	return jid
}

// Fixture generado con @whiskeysockets/baileys 7.0.0-rc14, usando
// generateWAMessageFromContent y después proto.Message.encode sobre el patrón
// viewOnceMessage solicitado. No es una traducción escrita a mano en Go: son
// los bytes que produjo protobufjs.
const fixtureBaileysBase64 = "qgLpAQrmAZoCBAoAEALqAtsBCg0KB1NISU4tTUQSACgAEhMKEUVsaWdlIHVuYSBvcGNpw7NuGhAKDlRvY2EgdW4gYm90w7NuMqIBCjIKC3F1aWNrX3JlcGx5EiN7ImRpc3BsYXlfdGV4dCI6IlBpbmciLCJpZCI6InBpbmcifQpqCgdjdGFfdXJsEl97ImRpc3BsYXlfdGV4dCI6IlJlcG9zaXRvcmlvIiwidXJsIjoiaHR0cHM6Ly9leGFtcGxlLmNvbSIsIm1lcmNoYW50X3VybCI6Imh0dHBzOi8vZXhhbXBsZS5jb20ifRgB"

func peticionFixture() Peticion {
	return Peticion{
		Clase:  ClaseBotones,
		Title:  "SHIN-MD",
		Body:   "Elige una opción",
		Footer: "Toca un botón",
		Buttons: []Button{
			{Name: "quick_reply", Params: `{"display_text":"Ping","id":"ping"}`},
			{Name: "cta_url", Params: `{"display_text":"Repositorio","url":"https://example.com","merchant_url":"https://example.com"}`},
		},
	}
}

func TestProtobufBaileysCampoPorCampoYBytePorByte(t *testing.T) {
	fixture, err := base64.StdEncoding.DecodeString(fixtureBaileysBase64)
	if err != nil {
		t.Fatal(err)
	}
	var desdeBaileys waE2E.Message
	if err := proto.Unmarshal(fixture, &desdeBaileys); err != nil {
		t.Fatalf("decodificar fixture de Baileys: %v", err)
	}
	nuestro, _, err := construirBaileys(peticionFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&desdeBaileys, nuestro) {
		t.Fatalf("el árbol protobuf no coincide\nBaileys: %s\nGo: %s", &desdeBaileys, nuestro)
	}
	bytesGo, err := proto.MarshalOptions{Deterministic: true}.Marshal(nuestro)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytesGo) != string(fixture) {
		t.Fatalf("mismo árbol pero bytes distintos\nBaileys=%s\nGo=%s", hex.EncodeToString(fixture), hex.EncodeToString(bytesGo))
	}

	// Los campos que causaban la diferencia de 0.23.1 quedan explicitados:
	// #37 viewOnce → #1 Message → #35 contexto + #45 interactivo.
	inner := nuestro.GetViewOnceMessage().GetMessage()
	if inner == nil || inner.GetMessageContextInfo().GetDeviceListMetadataVersion() != 2 {
		t.Fatal("falta viewOnce.message.messageContextInfo v2")
	}
	im := inner.GetInteractiveMessage()
	if im == nil || im.GetHeader().GetTitle() != "SHIN-MD" || im.GetHeader().GetHasMediaAttachment() {
		t.Fatal("header no coincide con Baileys")
	}
	if im.GetBody().GetText() != "Elige una opción" || im.GetFooter().GetText() != "Toca un botón" {
		t.Fatal("body/footer no coinciden con Baileys")
	}
	flow := im.GetNativeFlowMessage()
	if flow.GetMessageVersion() != 1 {
		t.Fatalf("messageVersion protobuf=%d; debe ser 1 (el 9 es del nodo binario)", flow.GetMessageVersion())
	}
	if len(flow.GetButtons()) != 2 || flow.GetButtons()[1].GetName() != "cta_url" {
		t.Fatal("botones protobuf distintos")
	}
}

func TestNativaSoloQuitaElEnvoltorio(t *testing.T) {
	baileys, _, err := construirBaileys(peticionFixture())
	if err != nil {
		t.Fatal(err)
	}
	nativa, _, err := construirNativo(peticionFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(baileys.GetViewOnceMessage().GetMessage(), nativa) {
		t.Fatal("la estrategia nativa debe llevar exactamente el mismo interior")
	}
	if nativa.GetViewOnceMessage() != nil || nativa.GetInteractiveMessage() == nil {
		t.Fatal("la estrategia nativa debe ser directa")
	}
}

func TestNodoBizCompletoYUnico(t *testing.T) {
	for _, jid := range []types.JID{jidPrivado(t), jidGrupo(t)} {
		nodes, err := nodosBiz(jid, "mixed", true)
		if err != nil {
			t.Fatal(err)
		}
		bizCount := 0
		for _, n := range nodes {
			if n.Tag == "biz" {
				bizCount++
			}
		}
		if bizCount != 1 {
			t.Fatalf("%s: hay %d nodos biz, debe haber uno", jid, bizCount)
		}
		biz := nodes[0]
		if biz.Attrs["actual_actors"] != "2" || biz.Attrs["host_storage"] != "2" {
			t.Fatalf("attrs biz incompletos: %v", biz.Attrs)
		}
		if _, ok := biz.Attrs["privacy_mode_ts"].(string); !ok {
			t.Fatalf("privacy_mode_ts debe viajar como texto: %T", biz.Attrs["privacy_mode_ts"])
		}
		kids, ok := biz.Content.([]binary.Node)
		if !ok || len(kids) != 2 {
			t.Fatalf("biz debe tener interactive + quality_control: %T %#v", biz.Content, biz.Content)
		}
		interactive, qc := kids[0], kids[1]
		if interactive.Tag != "interactive" || interactive.Attrs["type"] != "native_flow" || interactive.Attrs["v"] != "1" {
			t.Fatalf("interactive incorrecto: %#v", interactive)
		}
		flow := interactive.Content.([]binary.Node)[0]
		if flow.Attrs["v"] != "9" || flow.Attrs["name"] != "mixed" {
			t.Fatalf("native_flow incorrecto: %v", flow.Attrs)
		}
		decision, _ := qc.Attrs["decision_id"].(string)
		if qc.Tag != "quality_control" || len(decision) != 40 || qc.Attrs["source_type"] != "third_party" {
			t.Fatalf("quality_control incompleto: %#v", qc)
		}
		if _, err := hex.DecodeString(decision); err != nil {
			t.Fatalf("decision_id no es hexadecimal: %q", decision)
		}
		source := qc.Content.([]binary.Node)[0]
		if source.Tag != "decision_source" || source.Attrs["value"] != "df" {
			t.Fatalf("decision_source ausente: %#v", source)
		}
		if esPrivado(jid) && (len(nodes) != 2 || nodes[1].Tag != "bot") {
			t.Fatal("en privado deben viajar biz + bot")
		}
		if !esPrivado(jid) && len(nodes) != 1 {
			t.Fatal("en grupo solo debe viajar biz")
		}
	}
	a, _ := nodosBiz(jidGrupo(t), "mixed", false)
	b, _ := nodosBiz(jidGrupo(t), "mixed", false)
	qa := a[0].Content.([]binary.Node)[1].Attrs["decision_id"]
	qb := b[0].Content.([]binary.Node)[1].Attrs["decision_id"]
	if qa == qb {
		t.Fatal("cada envío necesita un decision_id nuevo")
	}
}

func TestMasDeTresBotonesLlevaBottomSheet(t *testing.T) {
	p := Peticion{Clase: ClaseMenu, Buttons: []Button{
		QuickReply("1", "uno"), QuickReply("2", "dos"), QuickReply("3", "tres"), QuickReply("4", "cuatro"),
	}}
	msg, _, err := construirBaileys(p)
	if err != nil {
		t.Fatal(err)
	}
	params := msg.GetViewOnceMessage().GetMessage().GetInteractiveMessage().GetNativeFlowMessage().GetMessageParamsJSON()
	var got struct {
		BottomSheet struct {
			Limit int `json:"in_thread_buttons_limit"`
		} `json:"bottom_sheet"`
		Multiple bool `json:"has_multiple_buttons"`
	}
	if err := json.Unmarshal([]byte(params), &got); err != nil {
		t.Fatalf("messageParamsJson inválido: %v (%q)", err, params)
	}
	if got.BottomSheet.Limit != 3 || !got.Multiple {
		t.Fatalf("bottom sheet incompleto: %+v", got)
	}
}

type envioGrabado struct {
	msg   *waE2E.Message
	nodes []binary.Node
}

type transporteFalso struct {
	errores []error
	envios  []envioGrabado
}

func (t *transporteFalso) Send(_ context.Context, _ types.JID, msg *waE2E.Message, nodes []binary.Node) error {
	t.envios = append(t.envios, envioGrabado{msg: msg, nodes: nodes})
	i := len(t.envios) - 1
	if i < len(t.errores) {
		return t.errores[i]
	}
	return nil
}

type logFalso struct{ lineas []string }

func (l *logFalso) Warn(s string) { l.lineas = append(l.lineas, s) }

func TestCadenaBaileysNativaTexto(t *testing.T) {
	transporte := &transporteFalso{errores: []error{errors.New("ack 479"), errors.New("ack 405"), nil}}
	log := &logFalso{}
	cadena := NuevaCadena(transporte, log, Baileys(), Nativa(), Texto())
	err := cadena.SendMenu(context.Background(), jidPrivado(t), "SHIN", "menú", "pie", QuickReply("Ping", "ping"))
	if err != nil {
		t.Fatal(err)
	}
	if len(transporte.envios) != 3 {
		t.Fatalf("envíos=%d; quería baileys→nativa→texto", len(transporte.envios))
	}
	if transporte.envios[0].msg.GetViewOnceMessage() == nil {
		t.Fatal("primero debe ir Baileys envuelto")
	}
	if transporte.envios[1].msg.GetInteractiveMessage() == nil || transporte.envios[1].msg.GetViewOnceMessage() != nil {
		t.Fatal("segundo debe ir nativo directo")
	}
	if !strings.Contains(transporte.envios[2].msg.GetConversation(), ".ping") || len(transporte.envios[2].nodes) != 0 {
		t.Fatal("tercero debe ser texto plano utilizable, sin nodos biz")
	}
	if len(log.lineas) != 2 || !strings.Contains(log.lineas[0], "probando nativa") || !strings.Contains(log.lineas[1], "probando texto") {
		t.Fatalf("log de degradación = %v", log.lineas)
	}
}

func TestCadenaSeDetieneEnPrimerExito(t *testing.T) {
	transporte := &transporteFalso{}
	cadena := NuevaCadena(transporte, nil, Baileys(), Nativa(), Texto())
	if err := cadena.SendButtons(context.Background(), jidGrupo(t), "", "hola", "", nil, QuickReply("OK", "ok")); err != nil {
		t.Fatal(err)
	}
	if len(transporte.envios) != 1 || transporte.envios[0].msg.GetViewOnceMessage() == nil {
		t.Fatalf("debía detenerse tras Baileys: %+v", transporte.envios)
	}
}

func TestListaCaeATextoConFilasAccionables(t *testing.T) {
	transporte := &transporteFalso{errores: []error{errors.New("lista no"), nil}}
	cadena := NuevaCadena(transporte, nil, Estrategias("")...)
	err := cadena.SendList(context.Background(), jidGrupo(t), "SHIN", "Elige", "Ver", "pie",
		ListSection{Title: "General", Rows: []ListRow{{ID: "ping", Title: "Ping", Desc: "latencia"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(transporte.envios) != 2 {
		t.Fatalf("envíos=%d; quería lista clásica → texto", len(transporte.envios))
	}
	plain := transporte.envios[1].msg.GetConversation()
	for _, trozo := range []string{"SHIN", "Elige", "Ping", "latencia", ".ping", "pie"} {
		if !strings.Contains(plain, trozo) {
			t.Fatalf("fallback no contiene %q:\n%s", trozo, plain)
		}
	}
}

func TestEstrategiasConfigurablesSiempreConTexto(t *testing.T) {
	got := Estrategias("nativa,native,desconocida,wrapped,baileys,nativo")
	nombres := make([]string, len(got))
	for i, e := range got {
		nombres[i] = e.Nombre()
	}
	if strings.Join(nombres, ",") != "nativa,baileys,texto" {
		t.Fatalf("orden=%v", nombres)
	}
}

func TestCatalogoDeBotonesYSingleSelect(t *testing.T) {
	casos := []struct {
		button Button
		name   string
		key    string
		value  string
	}{
		{CTACall("Llamar", "+52155"), "cta_call", "phone_number", "+52155"},
		{SendLocation("Mi ubicación"), "send_location", "display_text", "Mi ubicación"},
		{OpenWebview("Web", "https://x.dev"), "open_webview", "url", "https://x.dev"},
		{FlowButton("123", "tok", "Empezar"), "flow", "flow_id", "123"},
	}
	for _, tc := range casos {
		if tc.button.Name != tc.name {
			t.Fatalf("%s: name=%q", tc.name, tc.button.Name)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(tc.button.Params), &got); err != nil {
			t.Fatalf("%s: JSON: %v", tc.name, err)
		}
		if got[tc.key] != tc.value {
			t.Fatalf("%s: %s=%v, quería %q", tc.name, tc.key, got[tc.key], tc.value)
		}
	}

	selectButton := SingleSelect("Categorías", ListSection{Title: "Menú", Rows: []ListRow{
		{ID: "menu:economia", Title: "Economía", Desc: "monedas"},
	}})
	var selectJSON struct {
		Title    string `json:"title"`
		Sections []struct {
			Rows []struct {
				ID string `json:"id"`
			} `json:"rows"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(selectButton.Params), &selectJSON); err != nil {
		t.Fatal(err)
	}
	if selectButton.Name != "single_select" || selectJSON.Title != "Categorías" || selectJSON.Sections[0].Rows[0].ID != "menu:economia" {
		t.Fatalf("single_select incorrecto: %s %+v", selectButton.Name, selectJSON)
	}
}

func TestListaClasicaDirectaConNodoProductList(t *testing.T) {
	transporte := &transporteFalso{}
	cadena := NuevaCadena(transporte, nil, Estrategias("")...)
	err := cadena.SendList(context.Background(), jidGrupo(t), "SHIN", "elige", "Ver", "pie",
		ListSection{Title: "General", Rows: []ListRow{{ID: "ping", Title: "Ping", Desc: "latencia"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(transporte.envios) != 1 {
		t.Fatalf("envíos=%d; la lista rica se debe intentar una sola vez", len(transporte.envios))
	}
	envio := transporte.envios[0]
	lm := envio.msg.GetListMessage()
	if lm == nil || envio.msg.GetViewOnceMessage() != nil || envio.msg.GetDocumentWithCaptionMessage() != nil {
		t.Fatal("la lista debe ser ListMessage de primer nivel, sin wrapper")
	}
	if len(envio.nodes) != 0 {
		t.Fatal("AdditionalNodes duplicaría el biz que añade whatsmeow")
	}
	if lm.GetListType() != waE2E.ListMessage_SINGLE_SELECT || lm.GetSections()[0].GetRows()[0].GetRowID() != "ping" {
		t.Fatalf("payload de lista incorrecto: %v", lm)
	}
	if len(envio.msg.GetMessageContextInfo().GetMessageSecret()) != 32 {
		t.Fatal("la lista necesita secreto de 32 bytes para iOS")
	}
	if got := strings.ToLower(waE2E.ListMessage_ListType_name[int32(lm.GetListType())]); got != "product_list" {
		t.Fatalf("whatsmeow generaría biz type=%q, quería product_list", got)
	}
}

func TestCarruselUsaNativeFlow(t *testing.T) {
	carrusel, nombre, err := construirBaileys(Peticion{Clase: ClaseCarrusel, Body: "desliza", Cards: []Card{
		{Title: "Uno", Body: "1", Buttons: []Button{QuickReply("Ver", "uno")}},
		{Title: "Dos", Body: "2", Buttons: []Button{QuickReply("Ver", "dos")}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	exterior := carrusel.GetViewOnceMessage().GetMessage().GetInteractiveMessage()
	car := exterior.GetCarouselMessage()
	if nombre != "carousel" || car.GetMessageVersion() != 1 || len(car.GetCards()) != 2 {
		t.Fatalf("carrusel incorrecto: nombre=%q version=%d cards=%d", nombre, car.GetMessageVersion(), len(car.GetCards()))
	}
	if exterior.GetHeader() != nil || exterior.GetFooter() != nil || car.GetCards()[0].GetFooter() != nil {
		t.Fatal("los campos opcionales vacíos no deben viajar")
	}
	if car.GetCards()[0].GetNativeFlowMessage().GetButtons()[0].GetName() != "quick_reply" {
		t.Fatal("la tarjeta perdió sus botones")
	}
}

func TestFallbackConvierteIDsEnComandosEscribibles(t *testing.T) {
	menu := textoPlano(Peticion{
		Clase: ClaseMenu,
		Body:  "elige",
		Buttons: []Button{
			QuickReply("Economía", "menu:economia"),
			CTACall("Llamar", "+5215512345678"),
		},
	})
	for _, want := range []string{".menu economia", "+5215512345678"} {
		if !strings.Contains(menu, want) {
			t.Fatalf("fallback no contiene %q:\n%s", want, menu)
		}
	}
	if strings.Contains(menu, ".menu:economia") {
		t.Fatalf("el ID interno no es un comando escribible:\n%s", menu)
	}

	lista := textoPlano(Peticion{
		Clase: ClaseLista,
		Body:  "elige",
		Sections: []ListSection{{Rows: []ListRow{
			{ID: "play:nombre canción", Title: "Canción"},
		}}},
	})
	if !strings.Contains(lista, ".play nombre canción") {
		t.Fatalf("fila no accionable:\n%s", lista)
	}
}

func TestErrorDeConstruccionTambienCaeATexto(t *testing.T) {
	transporte := &transporteFalso{}
	cadena := NuevaCadena(transporte, nil, Baileys(), Nativa(), Texto())
	if err := cadena.SendMenu(context.Background(), jidGrupo(t), "SHIN", "menú sin botones", ""); err != nil {
		t.Fatal(err)
	}
	if len(transporte.envios) != 1 || transporte.envios[0].msg.GetConversation() == "" {
		t.Fatalf("el error de construcción no llegó al texto: %+v", transporte.envios)
	}
}

func TestCarruselPropagaNombreAlNodoBiz(t *testing.T) {
	transporte := &transporteFalso{}
	cadena := NuevaCadena(transporte, nil, Baileys())
	err := cadena.SendCarousel(context.Background(), jidGrupo(t), "desliza", "",
		Card{Title: "Uno", Body: "1", Buttons: []Button{QuickReply("Ver", "uno")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(transporte.envios) != 1 || len(transporte.envios[0].nodes) != 1 {
		t.Fatalf("envío de carrusel incompleto: %+v", transporte.envios)
	}
	biz := transporte.envios[0].nodes[0].Content.([]binary.Node)
	flow := biz[0].Content.([]binary.Node)[0]
	if flow.Attrs["name"] != "carousel" {
		t.Fatalf("nodo de carrusel name=%v", flow.Attrs["name"])
	}
}

func TestCadenaDevuelveTodosLosErrores(t *testing.T) {
	transporte := &transporteFalso{errores: []error{
		errors.New("fallo envuelto"), errors.New("fallo directo"), errors.New("fallo texto"),
	}}
	cadena := NuevaCadena(transporte, nil, Baileys(), Nativa(), Texto())
	err := cadena.SendMenu(context.Background(), jidGrupo(t), "SHIN", "menú", "", QuickReply("Ping", "ping"))
	if err == nil {
		t.Fatal("se esperaba el error de las tres estrategias")
	}
	for _, want := range []string{"baileys: fallo envuelto", "nativa: fallo directo", "texto: fallo texto"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error no contiene %q: %v", want, err)
		}
	}
}

func TestCarruselConservaImagenYParametrosDeAcciones(t *testing.T) {
	image := &waE2E.ImageMessage{URL: proto.String("https://example.com/card.jpg")}
	msg, _, err := construirNativo(Peticion{Clase: ClaseCarrusel, Body: "desliza", Cards: []Card{{
		Title: "Acciones", Body: "prueba", Image: image,
		Buttons: []Button{
			CTAURL("Web", "https://example.com"),
			CTACall("Llamar", "+5215512345678"),
			CTACopy("Copiar", "ABC-123"),
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	card := msg.GetInteractiveMessage().GetCarouselMessage().GetCards()[0]
	if !card.GetHeader().GetHasMediaAttachment() || card.GetHeader().GetImageMessage().GetURL() != image.GetURL() {
		t.Fatal("el carrusel perdió la imagen de la tarjeta")
	}
	buttons := card.GetNativeFlowMessage().GetButtons()
	if len(buttons) != 3 {
		t.Fatalf("botones=%d", len(buttons))
	}
	want := []struct {
		name  string
		field string
		value string
	}{
		{"cta_url", "url", "https://example.com"},
		{"cta_call", "phone_number", "+5215512345678"},
		{"cta_copy", "copy_code", "ABC-123"},
	}
	for i, expected := range want {
		var params map[string]any
		if err := json.Unmarshal([]byte(buttons[i].GetButtonParamsJSON()), &params); err != nil {
			t.Fatal(err)
		}
		if buttons[i].GetName() != expected.name || params[expected.field] != expected.value {
			t.Fatalf("acción %d perdida: %s %v", i, buttons[i].GetName(), params)
		}
	}
}
