package packs

type Definition struct {
	ID          string
	Name        string
	Description string
}

var definitions = map[string]Definition{
	"mobile-ad-monetization/v1": {
		ID:          "mobile-ad-monetization/v1",
		Name:        "Mobile ad monetization",
		Description: "Ad placements, frequency policies, feature switches, and environment bindings.",
	},
}

func Find(id string) (Definition, bool) {
	definition, ok := definitions[id]
	return definition, ok
}
