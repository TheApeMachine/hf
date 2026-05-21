package tokenizer

import "fmt"

/*
Tokenizer converts text to token IDs and token IDs back to text.
*/
type Tokenizer interface {
	Encode(text string) ([]int, error)
	Decode(tokenIDs []int, skipSpecialTokens bool) (string, error)
	VocabSize() int
	SpecialTokenIDs() []int
}

/*
Artifact binds a tokenizer implementation to the file it was loaded from.
*/
type Artifact struct {
	Source    Source
	Path      string
	Backend   string
	Tokenizer Tokenizer
}

/*
Source identifies a tokenizer file, either from a local directory or a Hub repo.
*/
type Source struct {
	Source   string
	File     string
	Cache    string
	Revision string
	RepoType string
}

func (source Source) WithDefaults() Source {
	if source.File == "" {
		source.File = "tokenizer.json"
	}

	return source
}

func (source Source) Key() string {
	source = source.WithDefaults()

	return fmt.Sprintf(
		"%s@%s:%s:%s",
		source.RepoType,
		source.Revision,
		source.Source,
		source.File,
	)
}
