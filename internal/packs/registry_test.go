package packs

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestRegistryReturnsMetadataSchemaDefaultsAndCapabilities(t *testing.T) {
	registry := MustNewRegistry(testDefinition("test-pack"))

	snapshot := registry.List()
	if snapshot.Revision != 1 || len(snapshot.Definitions) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	definition := snapshot.Definitions[0]
	if definition.Metadata.Name != "test-pack" || definition.Metadata.Version != "v1" {
		t.Fatalf("metadata = %#v", definition.Metadata)
	}
	if len(definition.Metadata.Capabilities) != 1 || definition.Metadata.Capabilities[0] != "environment_overrides" {
		t.Fatalf("capabilities = %#v", definition.Metadata.Capabilities)
	}
	entity := definition.Metadata.EntityTypes[0]
	if entity.DeletionPolicy != DeletionPolicyRestrict || len(entity.EnvironmentOverrideFields) != 1 || entity.EnvironmentOverrideFields[0] != "enabled" {
		t.Fatalf("entity metadata = %#v", entity)
	}
	field := definition.Schema.Entities[0].Fields[0]
	if string(field.Default) != "false" || field.Sensitivity != SensitivityPublic || field.UI.Control != "switch" {
		t.Fatalf("field schema = %#v", field)
	}
}

func TestEmptyRegistryStartsWithContractCompatibleRevision(t *testing.T) {
	registry := MustNewRegistry()
	snapshot := registry.List()
	if snapshot.Revision != 1 || len(snapshot.Definitions) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestRegistryReturnsDefensiveCopies(t *testing.T) {
	registry := MustNewRegistry(testDefinition("test-pack"))
	definition, _, err := registry.Get("test-pack", "v1")
	if err != nil {
		t.Fatal(err)
	}
	definition.Metadata.Capabilities[0] = "changed"
	definition.Schema.Entities[0].Fields[0].Default[0] = 't'

	again, _, err := registry.Get("test-pack", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if again.Metadata.Capabilities[0] != "environment_overrides" || string(again.Schema.Entities[0].Fields[0].Default) != "false" {
		t.Fatalf("registry was mutated: %#v", again)
	}
}

func TestRegistryDistinguishesLookupAndSchemaErrors(t *testing.T) {
	registry := MustNewRegistry(testDefinition("test-pack"))

	if _, _, err := registry.Get("missing-pack", "v1"); !errors.Is(err, ErrUnknownPack) {
		t.Fatalf("unknown pack error = %v", err)
	}
	if _, _, err := registry.Get("test-pack", "v2"); !errors.Is(err, ErrUnknownVersion) {
		t.Fatalf("unknown version error = %v", err)
	}
	requested := uint64(2)
	if _, _, err := registry.Schema("test-pack", "v1", &requested); !errors.Is(err, ErrSchemaIncompatible) {
		t.Fatalf("schema compatibility error = %v", err)
	}
}

func TestParseReference(t *testing.T) {
	reference, err := ParseReference("test-pack/v12")
	if err != nil || reference.Name != "test-pack" || reference.Version != "v12" || reference.String() != "test-pack/v12" {
		t.Fatalf("reference = %#v, err = %v", reference, err)
	}
	for _, value := range []string{"", "test-pack", "test-pack/v0", "test_pack/v1", "test-pack/v1/extra"} {
		if _, err := ParseReference(value); !errors.Is(err, ErrInvalidReference) {
			t.Fatalf("ParseReference(%q) error = %v", value, err)
		}
	}
}

func TestBuiltinRegistryResolvesCurrentManifestPack(t *testing.T) {
	definition, _, err := BuiltinRegistry().Resolve("mobile-ad-monetization/v1")
	if err != nil {
		t.Fatal(err)
	}
	if definition.Metadata.Name != "mobile-ad-monetization" || definition.Metadata.Version != "v1" || definition.Schema.Version != 1 {
		t.Fatalf("definition = %#v", definition)
	}
}

func TestRegistryConcurrentReadsAndRegistration(t *testing.T) {
	registry := MustNewRegistry(testDefinition("test-pack"))
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := 0; index < 100; index++ {
				_ = registry.List()
				if _, _, err := registry.Get("test-pack", "v1"); err != nil {
					t.Errorf("Get: %v", err)
				}
			}
		}()
	}
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			if err := registry.Register(testDefinition(fmt.Sprintf("test-pack-%d", index))); err != nil {
				t.Errorf("Register: %v", err)
			}
		}(index)
	}
	group.Wait()
	if got := len(registry.List().Definitions); got != 9 {
		t.Fatalf("definition count = %d, want 9", got)
	}
}

func TestRegistryRejectsUnknownEnvironmentOverrideField(t *testing.T) {
	definition := testDefinition("test-pack")
	definition.Metadata.EntityTypes[0].EnvironmentOverrideFields = []string{"missing"}
	registry, err := NewRegistry(definition)
	if registry != nil || !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("registry = %#v, err = %v", registry, err)
	}
}

func TestRegistryRejectsUnsupportedFieldType(t *testing.T) {
	definition := testDefinition("test-pack")
	definition.Schema.Entities[0].Fields[0].Type = FieldType("executable")
	registry, err := NewRegistry(definition)
	if registry != nil || !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("registry = %#v, err = %v", registry, err)
	}
}

func TestRegistryRejectsSchemaTypeAndMigrationMismatches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{
			name: "default type",
			mutate: func(definition *Definition) {
				definition.Schema.Entities[0].Fields[0].Default = json.RawMessage(`"false"`)
			},
		},
		{
			name: "enum type",
			mutate: func(definition *Definition) {
				definition.Schema.Entities[0].Fields[0].Validation.Enum = []json.RawMessage{json.RawMessage(`"yes"`)}
			},
		},
		{
			name: "length constraint on boolean",
			mutate: func(definition *Definition) {
				minimum := 1
				definition.Schema.Entities[0].Fields[0].Validation.MinLength = &minimum
			},
		},
		{
			name: "future migration target",
			mutate: func(definition *Definition) {
				definition.Schema.Migrations = []SchemaMigration{{FromVersion: 1, ToVersion: 2, Description: "Future schema."}}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := testDefinition("test-pack")
			test.mutate(&definition)
			registry, err := NewRegistry(definition)
			if registry != nil || !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("registry = %#v, err = %v", registry, err)
			}
		})
	}
}

func testDefinition(name string) Definition {
	return Definition{
		Metadata: Metadata{
			Name:         name,
			Version:      "v1",
			Description:  "A declarative test Pack.",
			Capabilities: []string{"environment_overrides"},
			EntityTypes: []EntityMetadata{{
				Name:                      "setting",
				Label:                     "Setting",
				Description:               "A test setting.",
				IDRule:                    IDRule{Pattern: "^[a-z][a-z0-9-]{0,62}$", MinLength: 1, MaxLength: 63},
				DeletionPolicy:            DeletionPolicyRestrict,
				EnvironmentOverrideFields: []string{"enabled"},
			}},
		},
		Schema: Schema{
			Version: 1,
			Entities: []EntitySchema{{
				Name: "setting",
				Fields: []FieldSchema{{
					Name:        "enabled",
					Type:        FieldTypeBoolean,
					Required:    true,
					Default:     json.RawMessage("false"),
					Sensitivity: SensitivityPublic,
					UI:          FieldUI{Label: "Enabled", Description: "Whether the setting is enabled.", Control: "switch", Group: "General", Order: 0},
					Validation:  FieldValidation{Enum: []json.RawMessage{}},
				}},
			}},
			Migrations: []SchemaMigration{},
		},
	}
}
