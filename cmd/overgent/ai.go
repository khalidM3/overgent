package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/hosted"
	protocoltypes "github.com/khalidM3/overgent/protocol/generated/go"
)

func runAI(ctx context.Context, paths config.Paths, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: overgent ai status|set|clear")
	}
	switch args[0] {
	case "status":
		fs := flag.NewFlagSet("ai status", flag.ContinueOnError)
		projectID := fs.String("project", "", "Project id")
		jsonOutput := fs.Bool("json", false, "write JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		client, selected, err := aiClient(ctx, paths, *projectID)
		if err != nil {
			return err
		}
		settings, err := client.AISettings(ctx, selected)
		if err != nil {
			return err
		}
		return writeAISettings(stdout, settings, *jsonOutput)
	case "set":
		return runAISet(ctx, paths, args[1:], stdin, stdout)
	case "clear":
		return runAIClear(ctx, paths, args[1:], stdout)
	default:
		return fmt.Errorf("unsupported ai command %q", args[0])
	}
}

func runAISet(ctx context.Context, paths config.Paths, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("ai set", flag.ContinueOnError)
	projectID := fs.String("project", "", "Project id")
	judgmentProvider := fs.String("judgment-provider", "", "anthropic, openai-compatible, or none")
	judgmentModel := fs.String("judgment-model", "", "judgment model")
	judgmentBaseURL := fs.String("judgment-base-url", "", "judgment provider origin")
	judgmentKeyStdin := fs.Bool("judgment-key-stdin", false, "read judgment key from stdin")
	judgmentKeyEnv := fs.String("judgment-key-env", "", "read judgment key from environment variable")
	embeddingProvider := fs.String("embedding-provider", "", "openai or deterministic")
	embeddingModel := fs.String("embedding-model", "", "embedding model")
	embeddingBaseURL := fs.String("embedding-base-url", "", "embedding provider origin")
	embeddingKeyStdin := fs.Bool("embedding-key-stdin", false, "read embedding key from stdin")
	embeddingKeyEnv := fs.String("embedding-key-env", "", "read embedding key from environment variable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*judgmentKeyStdin && *judgmentKeyEnv != "") || (*embeddingKeyStdin && *embeddingKeyEnv != "") {
		return errors.New("choose either stdin or an environment variable for each key")
	}
	if *judgmentKeyStdin && *embeddingKeyStdin {
		return errors.New("stdin can supply only one provider key per command")
	}
	client, selected, err := aiClient(ctx, paths, *projectID)
	if err != nil {
		return err
	}
	current, err := client.AISettings(ctx, selected)
	if err != nil {
		return err
	}
	write := settingsWrite(current)
	if *judgmentProvider != "" {
		write.Judgment.Provider = *judgmentProvider
	}
	if *judgmentModel != "" {
		write.Judgment.Model = *judgmentModel
	}
	if *judgmentBaseURL != "" {
		write.Judgment.BaseUrl = judgmentBaseURL
	}
	if *embeddingProvider != "" {
		write.Embeddings.Provider = *embeddingProvider
	}
	if *embeddingModel != "" {
		write.Embeddings.Model = *embeddingModel
	}
	if *embeddingBaseURL != "" {
		write.Embeddings.BaseUrl = embeddingBaseURL
	}
	judgmentKey, judgmentSet, err := readProviderKey(stdin, *judgmentKeyStdin, *judgmentKeyEnv)
	if err != nil {
		return err
	}
	embeddingKey, embeddingSet, err := readProviderKey(stdin, *embeddingKeyStdin, *embeddingKeyEnv)
	if err != nil {
		return err
	}
	if judgmentSet {
		var value protocoltypes.PutProjectAISettingsJSONBody_Judgment_ApiKey
		if err := value.FromPutProjectAISettingsJSONBodyJudgmentApiKey1(judgmentKey); err != nil {
			return err
		}
		write.Judgment.ApiKey = &value
	}
	if embeddingSet {
		var value protocoltypes.PutProjectAISettingsJSONBody_Embeddings_ApiKey
		if err := value.FromPutProjectAISettingsJSONBodyEmbeddingsApiKey1(embeddingKey); err != nil {
			return err
		}
		write.Embeddings.ApiKey = &value
	}
	updated, err := client.PutAISettings(ctx, selected, write)
	if err != nil {
		return err
	}
	return writeAISettings(stdout, updated, false)
}

func runAIClear(ctx context.Context, paths config.Paths, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("ai clear", flag.ContinueOnError)
	projectID := fs.String("project", "", "Project id")
	judgment := fs.Bool("judgment", false, "disable judgment and clear its key")
	embeddings := fs.Bool("embeddings", false, "use deterministic embeddings and clear their key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*judgment && !*embeddings {
		*judgment, *embeddings = true, true
	}
	client, selected, err := aiClient(ctx, paths, *projectID)
	if err != nil {
		return err
	}
	current, err := client.AISettings(ctx, selected)
	if err != nil {
		return err
	}
	write := settingsWrite(current)
	if *judgment {
		write.Judgment.Provider = "none"
		write.Judgment.Model = "none"
		var empty protocoltypes.PutProjectAISettingsJSONBody_Judgment_ApiKey
		if err := empty.FromPutProjectAISettingsJSONBodyJudgmentApiKey1(""); err != nil {
			return err
		}
		write.Judgment.ApiKey = &empty
	}
	if *embeddings {
		write.Embeddings.Provider = "deterministic"
		write.Embeddings.Model = "deterministic-v1"
		var empty protocoltypes.PutProjectAISettingsJSONBody_Embeddings_ApiKey
		if err := empty.FromPutProjectAISettingsJSONBodyEmbeddingsApiKey1(""); err != nil {
			return err
		}
		write.Embeddings.ApiKey = &empty
	}
	updated, err := client.PutAISettings(ctx, selected, write)
	if err != nil {
		return err
	}
	return writeAISettings(stdout, updated, false)
}

func aiClient(ctx context.Context, paths config.Paths, explicitProjectID string) (*hosted.Client, string, error) {
	cfg, err := config.Load(paths)
	if err != nil {
		return nil, "", err
	}
	projectID := explicitProjectID
	if projectID == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return nil, "", fmt.Errorf("resolve current directory: %w", cwdErr)
		}
		workspace, ok := workspaceForCommand(cfg, cwd)
		if !ok {
			return nil, "", errors.New("current directory is not inside a registered workspace; pass --project")
		}
		projectID = workspace.ProjectID
	}
	// AI settings are a property of the Project (ADR-073), so they are written
	// to the backend that Project lives on rather than to a profile-wide one.
	backend, bound := cfg.BackendForProject(projectID)
	if !bound || backend.DeviceID == "" {
		return nil, "", errors.New("ai settings require an enrolled Project")
	}
	token, err := credential.Get(ctx, backend.DeviceID)
	if err != nil {
		return nil, "", err
	}
	client, err := hosted.New(backend.APIBaseURL, token)
	return client, projectID, err
}

// This mirrors the longest registered-root match used by internal/app without
// changing app.go, which belongs to the project-backend-binding lane.
func workspaceForCommand(cfg config.Config, cwd string) (config.Workspace, bool) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return config.Workspace{}, false
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return config.Workspace{}, false
	}
	var selected config.Workspace
	for _, candidate := range cfg.Workspaces {
		root, rootErr := filepath.EvalSymlinks(candidate.Root)
		if rootErr != nil {
			continue
		}
		relative, relErr := filepath.Rel(root, abs)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && len(root) > len(selected.Root) {
			selected = candidate
		}
	}
	return selected, selected.Root != ""
}

func settingsWrite(current hosted.AISettings) hosted.AISettingsWrite {
	var write hosted.AISettingsWrite
	write.Judgment.Provider = current.Judgment.Provider
	write.Judgment.Model = current.Judgment.Model
	write.Judgment.BaseUrl = current.Judgment.BaseURL
	write.Embeddings.Provider = current.Embeddings.Provider
	write.Embeddings.Model = current.Embeddings.Model
	write.Embeddings.Dimensions = current.Embeddings.Dimensions
	write.Embeddings.BaseUrl = current.Embeddings.BaseURL
	return write
}

func readProviderKey(stdin io.Reader, fromStdin bool, envName string) (string, bool, error) {
	if envName != "" {
		value, ok := os.LookupEnv(envName)
		if !ok {
			return "", false, fmt.Errorf("environment variable %s is not set", envName)
		}
		return value, true, nil
	}
	if !fromStdin {
		return "", false, nil
	}
	value, err := io.ReadAll(io.LimitReader(stdin, 514))
	if err != nil {
		return "", false, fmt.Errorf("read provider key: %w", err)
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(value), "\n"), "\r"), true, nil
}

func writeAISettings(stdout io.Writer, settings hosted.AISettings, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(stdout).Encode(settings)
	}
	_, err := fmt.Fprintf(stdout, "AI: judgment=%s embeddings=%s\n", settings.Effective.Judgment, settings.Effective.Embeddings)
	return err
}

func printAIDoctor(ctx context.Context, paths config.Paths, stdout io.Writer) error {
	client, projectID, err := aiClient(ctx, paths, "")
	if err != nil {
		_, writeErr := fmt.Fprintln(stdout, "AI: judgment=unavailable embeddings=unavailable")
		return writeErr
	}
	settings, err := client.AISettings(ctx, projectID)
	if err != nil {
		_, writeErr := fmt.Fprintln(stdout, "AI: judgment=unavailable embeddings=unavailable")
		return writeErr
	}
	return writeAISettings(stdout, settings, false)
}
