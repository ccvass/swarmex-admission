package admission

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yamlContent := `validations:
  - name: require-owner
    require_labels:
      - com.example.owner
  - name: no-latest
    forbid_images:
      - ".*:latest$"
  - name: max-replicas
    max_replicas: 20
mutations:
  - name: add-monitoring
    match_label: swarmex.monitored
    add_labels:
      prometheus.scrape: "true"
      prometheus.port: "9090"
`
	f, err := os.CreateTemp("", "admission-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlContent)
	f.Close()

	cfg, err := LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if len(cfg.Validations) != 3 {
		t.Fatalf("expected 3 validations, got %d", len(cfg.Validations))
	}
	if cfg.Validations[0].Name != "require-owner" {
		t.Errorf("expected require-owner, got %s", cfg.Validations[0].Name)
	}
	if len(cfg.Validations[0].RequireLabels) != 1 {
		t.Errorf("expected 1 required label, got %d", len(cfg.Validations[0].RequireLabels))
	}
	if cfg.Validations[2].MaxReplicas == nil || *cfg.Validations[2].MaxReplicas != 20 {
		t.Errorf("expected max_replicas=20")
	}
	if len(cfg.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %d", len(cfg.Mutations))
	}
	if len(cfg.Mutations[0].AddLabels) != 2 {
		t.Errorf("expected 2 add_labels, got %d", len(cfg.Mutations[0].AddLabels))
	}
}
