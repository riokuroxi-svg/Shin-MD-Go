// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"context"
	"sync"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// fakeIDs es una base de identidades en memoria (la real es SQLite).
type fakeIDs struct {
	mu     sync.Mutex
	porLID map[string]string
	porPN  map[string]string
	pares  int
}

func newFakeIDs() *fakeIDs {
	return &fakeIDs{porLID: map[string]string{}, porPN: map[string]string{}}
}

func (f *fakeIDs) PutIdentity(_ context.Context, lid, pn string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.porLID[lid] = pn
	f.porPN[pn] = lid
	f.pares++
	return nil
}

func (f *fakeIDs) PNForLID(_ context.Context, lid string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.porLID[lid], nil
}

func (f *fakeIDs) LIDForPN(_ context.Context, pn string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.porPN[pn], nil
}

func TestResolverFromAlternate(t *testing.T) {
	db := newFakeIDs()
	r := NewResolver(db, nil)
	ctx := context.Background()

	lid, _ := types.ParseJID("123456789012345@lid")
	pn, _ := types.ParseJID("5215512345678@s.whatsapp.net")

	// El mensaje llega por LID y trae el teléfono en el alternativo: la llave
	// tiene que ser el teléfono, y la pareja queda guardada.
	id := r.Resolve(ctx, lid, pn)
	if id.Key != pn.String() {
		t.Fatalf("llave = %q, se esperaba %q", id.Key, pn.String())
	}
	if !id.Known() {
		t.Fatal("debería considerarse completa")
	}
	if db.pares != 1 {
		t.Fatalf("pares guardados = %d", db.pares)
	}
}

func TestResolverLearnedFromDatabase(t *testing.T) {
	db := newFakeIDs()
	ctx := context.Background()
	lid, _ := types.ParseJID("999@lid")
	pn, _ := types.ParseJID("5215500000000@s.whatsapp.net")
	_ = db.PutIdentity(ctx, lid.String(), pn.String())

	// Resolver nuevo (como tras un reinicio): sin memoria, solo con la base.
	r := NewResolver(db, nil)
	id := r.Resolve(ctx, lid, types.EmptyJID)
	if id.Key != pn.String() {
		t.Fatalf("llave = %q, se esperaba el teléfono %q", id.Key, pn.String())
	}
}

func TestResolverUnknownLIDKeepsWorking(t *testing.T) {
	r := NewResolver(newFakeIDs(), nil)
	ctx := context.Background()

	// Un LID que nunca se ha visto: no se puede resolver (WhatsApp no expone
	// LID → teléfono), pero el bot tiene que seguir funcionando con esa llave.
	lid, _ := types.ParseJID("777777777777@lid")
	id := r.Resolve(ctx, lid, types.EmptyJID)

	if id.Key != lid.String() {
		t.Fatalf("llave = %q, se esperaba el LID", id.Key)
	}
	if id.Known() {
		t.Fatal("no puede considerarse completa sin teléfono")
	}
	if id.PN.IsEmpty() == false {
		t.Fatal("no debería haber inventado un teléfono")
	}
}

func TestResolverPhoneOnly(t *testing.T) {
	r := NewResolver(newFakeIDs(), nil)
	ctx := context.Background()

	pn, _ := types.ParseJID("5215511111111@s.whatsapp.net")
	id := r.Resolve(ctx, pn, types.EmptyJID)
	if id.Key != pn.String() || !id.Known() {
		t.Fatalf("identidad = %+v", id)
	}
	if !id.LID.IsEmpty() {
		t.Fatal("sin red no debería haber LID")
	}
}

func TestSamePersonAcrossAddressing(t *testing.T) {
	db := newFakeIDs()
	r := NewResolver(db, nil)
	ctx := context.Background()

	lid, _ := types.ParseJID("555@lid")
	pn, _ := types.ParseJID("5215522222222@s.whatsapp.net")
	otro, _ := types.ParseJID("5215533333333@s.whatsapp.net")

	if !r.SamePerson(ctx, lid, pn, pn, types.EmptyJID) {
		t.Fatal("el mismo usuario por LID y por teléfono debería coincidir")
	}
	if r.SamePerson(ctx, lid, pn, otro, types.EmptyJID) {
		t.Fatal("dos usuarios distintos no deberían coincidir")
	}
}

func TestResolverCacheDoesNotChangeAnswer(t *testing.T) {
	db := newFakeIDs()
	r := NewResolver(db, nil)
	ctx := context.Background()

	lid, _ := types.ParseJID("888@lid")
	pn, _ := types.ParseJID("5215544444444@s.whatsapp.net")

	first := r.Resolve(ctx, lid, pn)
	second := r.Resolve(ctx, lid, types.EmptyJID) // ahora sin alternativo: usa caché
	if first.Key != second.Key {
		t.Fatalf("la caché cambió la respuesta: %q vs %q", first.Key, second.Key)
	}
}

func TestIsLID(t *testing.T) {
	lid, _ := types.ParseJID("123@lid")
	hosted, _ := types.ParseJID("123@hosted.lid")
	pn, _ := types.ParseJID("5215512345678@s.whatsapp.net")
	group, _ := types.ParseJID("123-456@g.us")

	if !IsLID(lid) || !IsLID(hosted) {
		t.Fatal("los @lid deberían detectarse como LID")
	}
	if IsLID(pn) || IsLID(group) {
		t.Fatal("un teléfono o un grupo no son LID")
	}
}
