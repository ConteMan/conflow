package packs

// BuiltinRegistry contains Pack declarations compiled into the Conflow binary.
// Domain entities for mobile advertising are intentionally introduced by Spec
// 006; this contract-only Pack is still resolvable by existing project files.
func BuiltinRegistry() *Registry {
	return MustNewRegistry(Definition{
		Metadata: Metadata{
			Name:         "mobile-ad-monetization",
			Version:      "v1",
			Description:  "Versioned contract for mobile advertising configuration.",
			Capabilities: []string{},
			EntityTypes:  []EntityMetadata{},
		},
		Schema: Schema{
			Version:    1,
			Entities:   []EntitySchema{},
			Migrations: []SchemaMigration{},
		},
	})
}
