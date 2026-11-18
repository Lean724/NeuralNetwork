package storage

import (
	"path/filepath"
	"testing"

	"neuralnetwork/internal/models"
)

func TestSQLitePersistenceNodesAndSettings(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test_neural.db")
	store, err := NewStore(dbFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	fx, fy, fz := 12.5, -40.0, 88.25
	custom := models.NodeCustomization{
		IP:       "192.168.1.50",
		MAC:      "dc:a6:32:4a:11:08",
		Alias:    "Servidor Principal",
		Category: "server",
		Pinned:   true,
		FX:       &fx,
		FY:       &fy,
		FZ:       &fz,
	}
	if err := store.UpsertNodeCustomization(custom); err != nil {
		t.Fatalf("UpsertNodeCustomization failed: %v", err)
	}

	settings := models.AppSettings{
		GroupExternal: true,
		NodeTTLSec:    240,
		BloomStrength: 1.8,
		CaptureMode:   "simulation",
		Interface:     "eth0",
	}
	if err := store.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reabrir desde disco para validar persistencia real
	reopened, err := NewStore(dbFile)
	if err != nil {
		t.Fatalf("Reopen NewStore failed: %v", err)
	}
	defer reopened.Close()

	nodes, err := reopened.GetKnownNodes()
	if err != nil {
		t.Fatalf("GetKnownNodes failed: %v", err)
	}
	got, ok := nodes["192.168.1.50"]
	if !ok {
		t.Fatalf("nodo 192.168.1.50 no encontrado tras reabrir")
	}
	if got.Alias != "Servidor Principal" || !got.Pinned || got.Category != "server" {
		t.Errorf("datos inesperados del nodo: %+v", got)
	}
	if got.FX == nil || *got.FX != fx || got.FY == nil || *got.FY != fy || got.FZ == nil || *got.FZ != fz {
		t.Errorf("coordenadas fijadas no persistieron correctamente: %+v", got)
	}

	loadedSettings, err := reopened.GetSettings(models.AppSettings{})
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if !loadedSettings.GroupExternal || loadedSettings.NodeTTLSec != 240 || loadedSettings.CaptureMode != "simulation" {
		t.Errorf("configuración persistida incorrecta: %+v", loadedSettings)
	}

	if err := reopened.DeleteNodeCustomization("192.168.1.50"); err != nil {
		t.Fatalf("DeleteNodeCustomization failed: %v", err)
	}
	nodesAfterDelete, _ := reopened.GetKnownNodes()
	if _, exists := nodesAfterDelete["192.168.1.50"]; exists {
		t.Errorf("el nodo debería haber sido eliminado")
	}
}
