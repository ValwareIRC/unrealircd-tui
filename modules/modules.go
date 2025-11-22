package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GitHubItem represents a GitHub repository item
type GitHubItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

// Module represents a third-party module
type Module struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Source         string   `json:"source"`
	PostInstallText []string `json:"post_install_text"`
}

// FetchRepoContents fetches repository contents from GitHub API
func FetchRepoContents(owner, repo, path, ref string) ([]GitHubItem, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, ref)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var items []GitHubItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	return items, nil
}

// FetchFileContent fetches raw file content from a URL
func FetchFileContent(downloadURL string) (string, error) {
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch file: HTTP %d", resp.StatusCode)
	}

	content := make([]byte, 0)
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			content = append(content, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	return string(content), nil
}

// ParseModulesList parses a modules list from text content
func ParseModulesList(content string) ([]Module, error) {
	var modules []Module
	lines := strings.Split(content, "\n")

	var currentModule *Module
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "name:") {
			if currentModule != nil {
				modules = append(modules, *currentModule)
			}
			currentModule = &Module{}
			currentModule.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") && currentModule != nil {
			currentModule.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		} else if strings.HasPrefix(line, "source:") && currentModule != nil {
			currentModule.Source = strings.TrimSpace(strings.TrimPrefix(line, "source:"))
		} else if strings.HasPrefix(line, "post_install:") && currentModule != nil {
			text := strings.TrimSpace(strings.TrimPrefix(line, "post_install:"))
			currentModule.PostInstallText = append(currentModule.PostInstallText, text)
		}
	}

	if currentModule != nil {
		modules = append(modules, *currentModule)
	}

	return modules, nil
}

// ParseModulesSources parses the modules.sources.list file
func ParseModulesSources() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	sourcesFile := filepath.Join(home, ".unrealircd", "modules.sources.list")
	content, err := os.ReadFile(sourcesFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default sources
			return []string{
				"https://raw.githubusercontent.com/unrealircd/unrealircd-contrib/unreal6/files/modules.list",
			}, nil
		}
		return nil, err
	}

	var sources []string
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			sources = append(sources, line)
		}
	}

	return sources, nil
}

// FormatModuleDetails formats module details for display
func FormatModuleDetails(mod Module) string {
	details := fmt.Sprintf("[blue]Name:[white] %s\n", mod.Name)
	details += fmt.Sprintf("[blue]Description:[white] %s\n", mod.Description)

	if len(mod.PostInstallText) > 0 {
		details += "\n[green]Post-Install Instructions:[white]\n"
		for _, text := range mod.PostInstallText {
			details += fmt.Sprintf("• %s\n", text)
		}
	}

	return details
}

// InstallModule downloads and installs a module
func InstallModule(sourceDir, buildDir, downloadURL, filename string) error {
	// Download the module
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download module: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to download module: HTTP %d", resp.StatusCode)
	}

	content := make([]byte, 0)
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			content = append(content, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	// Save to third party modules directory
	thirdDir := filepath.Join(sourceDir, "src", "modules", "third")
	if err := os.MkdirAll(thirdDir, 0755); err != nil {
		return fmt.Errorf("failed to create third party modules directory: %v", err)
	}

	filePath := filepath.Join(thirdDir, filename)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return fmt.Errorf("failed to save module: %v", err)
	}

	return nil
}

// UpdateModsConf updates mods.conf to include a module
func UpdateModsConf(buildDir, moduleName string) error {
	confFile := filepath.Join(buildDir, "conf", "mods.conf")

	// Read existing content
	content, err := os.ReadFile(confFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if already included
	loadLine := fmt.Sprintf("loadmodule \"%s\";", moduleName)
	if strings.Contains(string(content), loadLine) {
		return nil
	}

	// Add the loadmodule line
	newContent := string(content)
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += loadLine + "\n"

	return os.WriteFile(confFile, []byte(newContent), 0644)
}