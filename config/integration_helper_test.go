package hfconfig

import (
	"context"

	"github.com/theapemachine/manifesto/compiler"
)

/*
compileLoweredForTest runs the full ProgramCompiler pipeline against a
synthetic program manifest that includes the rendered block YAML under
the name "model". Used by integration_test.go to drive the same code
path `caramba chat` exercises.
*/
func compileLoweredForTest(blockYAML string) (*compiler.CompileOutput, error) {
	programYAML := []byte(`kind: program
name: integration
include:
  model: hf://example/integration-test
`)

	resolver := &fixedResolver{payload: []byte(blockYAML)}

	programCompiler, err := compiler.NewProgramCompiler(compiler.NewPool(nil))

	if err != nil {
		return nil, err
	}

	programCompiler = programCompiler.WithIncludeResolver(resolver)

	return programCompiler.CompileAssets(context.Background(), compiler.CompileInput{
		ProgramYAML: programYAML,
	}, nil)
}

type fixedResolver struct {
	payload []byte
}

func (resolver *fixedResolver) ResolveInclude(ctx context.Context, include compiler.IncludeSource) ([]byte, error) {
	_ = ctx
	_ = include
	return resolver.payload, nil
}
