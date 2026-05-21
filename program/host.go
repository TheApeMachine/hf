package program

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/theapemachine/hf/hub"
	"github.com/theapemachine/hf/tokenizer"
	"github.com/theapemachine/manifesto/runtime"
)

/*
Host implements manifesto runtime host operations for program execution.
*/
type Host struct {
	stdin     io.Reader
	hubConfig *hub.HubConfig
}

/*
HostOptions configures program host behavior.
*/
type HostOptions struct {
	Stdin     io.Reader
	HubConfig *hub.HubConfig
}

/*
NewHost constructs host-side IO and tokenizer behavior.
*/
func NewHost(options HostOptions) *Host {
	stdin := options.Stdin

	if stdin == nil {
		stdin = os.Stdin
	}

	hubConfig := options.HubConfig

	if hubConfig == nil {
		hubConfig = hub.DefaultHubConfig()
	}

	return &Host{
		stdin:     stdin,
		hubConfig: hubConfig,
	}
}

/*
ReadLine reads one line from the program stdin source.
*/
func (host *Host) ReadLine(ctx context.Context) (string, error) {
	_ = ctx

	reader, ok := host.stdin.(*bufio.Reader)

	if !ok {
		reader = bufio.NewReader(host.stdin)
	}

	line, err := reader.ReadString('\n')

	if err != nil && err != io.EOF {
		return "", err
	}

	if err == io.EOF && line == "" {
		return "", io.EOF
	}

	return stringsTrimRightNewline(line), nil
}

/*
EmitToken decodes one token id and writes it to stdout.
*/
func (host *Host) EmitToken(ctx context.Context, request runtime.EmitTokenRequest) error {
	artifact, err := host.loadTokenizer(ctx, request.Tokenizer, request.TokenizerFile)

	if err != nil {
		return err
	}

	text, err := artifact.Tokenizer.Decode([]int{request.TokenID}, true)

	if err != nil {
		return fmt.Errorf("program host: decode token: %w", err)
	}

	_, err = fmt.Fprint(os.Stdout, text)

	return err
}

/*
Encode tokenizes program text, optionally applying a chat template.
*/
func (host *Host) Encode(ctx context.Context, request runtime.EncodeRequest) ([]int, error) {
	artifact, err := host.loadTokenizer(ctx, request.Tokenizer, request.TokenizerFile)

	if err != nil {
		return nil, err
	}

	text := request.Text

	if request.ApplyChatTemplate {
		metadata, metadataErr := tokenizer.LoadMetadata(ctx, tokenizerSource(
			request.Tokenizer,
			request.TokenizerFile,
			host.hubConfig.CacheDir,
		))

		if metadataErr != nil {
			return nil, metadataErr
		}

		text, err = metadata.ApplyChatTemplate(text)

		if err != nil {
			return nil, err
		}
	}

	tokenIDs, err := artifact.Tokenizer.Encode(text)

	if err != nil {
		return nil, fmt.Errorf("program host: encode: %w", err)
	}

	if request.MaxLength > 0 && len(tokenIDs) > request.MaxLength {
		tokenIDs = tokenIDs[:request.MaxLength]
	}

	if request.PadTokenID != 0 && request.MaxLength > 0 {
		for len(tokenIDs) < request.MaxLength {
			tokenIDs = append(tokenIDs, request.PadTokenID)
		}
	}

	return tokenIDs, nil
}

func (host *Host) loadTokenizer(
	ctx context.Context,
	tokenizerName string,
	tokenizerFile string,
) (*tokenizer.Artifact, error) {
	return tokenizer.Load(ctx, tokenizerSource(tokenizerName, tokenizerFile, host.hubConfig.CacheDir))
}

func tokenizerSource(tokenizerName, tokenizerFile, cacheDir string) tokenizer.Source {
	source := tokenizer.Source{
		Source: tokenizerName,
		Cache:  cacheDir,
	}

	if tokenizerFile != "" {
		source.File = tokenizerFile
	}

	return source.WithDefaults()
}

func stringsTrimRightNewline(text string) string {
	if len(text) == 0 {
		return text
	}

	if text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
	}

	if len(text) > 0 && text[len(text)-1] == '\r' {
		text = text[:len(text)-1]
	}

	return text
}
