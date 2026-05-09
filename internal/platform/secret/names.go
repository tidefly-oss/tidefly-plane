package secret

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// adjectives and nouns for readable random names (docker-style)
var adjectives = []string{
	"brave", "calm", "dark", "eager", "fast", "gentle", "happy", "icy",
	"jolly", "kind", "lively", "merry", "noble", "odd", "proud", "quiet",
	"rapid", "sharp", "tidy", "unique", "vivid", "warm", "xenial", "young", "zesty",
	"amber", "azure", "brisk", "crisp", "daring", "elegant", "fierce", "grand",
	"hollow", "ivory", "jade", "keen", "lunar", "misty", "neon", "olive",
	"polished", "quirky", "rustic", "silver", "teal", "ultra", "violet", "wild",
}

var nouns = []string{
	"atlas", "birch", "coast", "dawn", "echo", "flare", "grove", "harbor",
	"inlet", "jungle", "knoll", "lagoon", "marsh", "nexus", "orbit", "peak",
	"quartz", "ridge", "storm", "tide", "umbra", "vale", "wave", "xenon",
	"yonder", "zenith", "beacon", "cedar", "delta", "ember", "frost", "gale",
	"haven", "iris", "jasper", "kite", "larch", "maple", "nova", "opal",
	"prism", "quill", "raven", "slate", "terra", "upper", "vortex", "willow",
}

// GenerateName returns a random human-readable name like "brave-atlas"
// with an optional prefix, e.g. GenerateName("pg") → "pg-brave-atlas"
func GenerateName(prefix string) (string, error) {
	adj, err := randomChoice(adjectives)
	if err != nil {
		return "", fmt.Errorf("generate name: %w", err)
	}
	noun, err := randomChoice(nouns)
	if err != nil {
		return "", fmt.Errorf("generate name: %w", err)
	}
	if prefix != "" {
		return fmt.Sprintf("%s-%s-%s", prefix, adj, noun), nil
	}
	return fmt.Sprintf("%s-%s", adj, noun), nil
}

func randomChoice(list []string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(list))))
	if err != nil {
		return "", err
	}
	return list[n.Int64()], nil
}
