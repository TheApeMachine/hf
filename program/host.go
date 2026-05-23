package program

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/theapemachine/hf/hub"
	"github.com/theapemachine/hf/tokenizer"
	"github.com/theapemachine/manifesto/runtime"
)

/*
Host implements manifesto runtime host operations for program execution.
*/
type Host struct {
	stdinReader *bufio.Reader
	hubConfig   *hub.HubConfig
	mu          sync.Mutex
	metadata    map[string]*tokenizer.Metadata
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

	reader, ok := stdin.(*bufio.Reader)

	if !ok {
		reader = bufio.NewReader(stdin)
	}

	return &Host{
		stdinReader: reader,
		hubConfig:   hubConfig,
		metadata:    make(map[string]*tokenizer.Metadata),
	}
}

/*
ReadLine reads one line from the program stdin source.
*/
func (host *Host) ReadLine(ctx context.Context) (string, error) {
	_ = ctx

	line, err := host.stdinReader.ReadString('\n')

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
		metadata, metadataErr := host.loadMetadata(ctx, request.Tokenizer, request.TokenizerFile)

		if metadataErr != nil {
			return nil, metadataErr
		}

		if request.ChatContinuation {
			text, err = metadata.ApplyChatContinuation(text)
		} else {
			text, err = metadata.ApplyChatTemplate(text)
		}

		if err != nil {
			return nil, err
		}
	}

	tokenIDs, err := artifact.Tokenizer.Encode(text)

	if err != nil {
		return nil, fmt.Errorf("program host: encode: %w", err)
	}

	padTokenID, err := host.encodePadTokenID(ctx, request)

	if err != nil {
		return nil, err
	}

	if request.MaxLength > 0 && len(tokenIDs) > request.MaxLength {
		tokenIDs = tokenIDs[:request.MaxLength]
	}

	if padTokenID != 0 && request.MaxLength > 0 {
		for len(tokenIDs) < request.MaxLength {
			tokenIDs = append(tokenIDs, padTokenID)
		}
	}

	return tokenIDs, nil
}

func (host *Host) encodePadTokenID(
	ctx context.Context,
	request runtime.EncodeRequest,
) (int, error) {
	if request.PadTokenID != 0 {
		return request.PadTokenID, nil
	}

	metadata, err := host.loadMetadata(ctx, request.Tokenizer, request.TokenizerFile)

	if err != nil {
		return 0, err
	}

	return metadata.PadTokenID, nil
}

func (host *Host) loadTokenizer(
	ctx context.Context,
	tokenizerName string,
	tokenizerFile string,
) (*tokenizer.Artifact, error) {
	return tokenizer.Load(ctx, tokenizerSource(tokenizerName, tokenizerFile, host.hubConfig.CacheDir))
}

func (host *Host) loadMetadata(
	ctx context.Context,
	tokenizerName string,
	tokenizerFile string,
) (*tokenizer.Metadata, error) {
	source := tokenizerSource(tokenizerName, tokenizerFile, host.hubConfig.CacheDir)
	key := source.Key()

	host.mu.Lock()
	cached, ok := host.metadata[key]
	host.mu.Unlock()

	if ok {
		return cached, nil
	}

	metadata, err := tokenizer.LoadMetadata(ctx, source)

	if err != nil {
		return nil, err
	}

	host.mu.Lock()
	host.metadata[key] = metadata
	host.mu.Unlock()

	return metadata, nil
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
