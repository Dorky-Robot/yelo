package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dorkyrobot/yelo/internal/aws"
)

func fetchList(client aws.S3Client, bucket, prefix string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		items, err := client.ListObjects(ctx, bucket, prefix, false)
		return listResultMsg{items: items, err: err}
	}
}

func fetchDetail(client aws.S3Client, bucket, key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		info, err := client.HeadObject(ctx, bucket, key)
		return detailResultMsg{info: info, err: err}
	}
}

func fetchBuckets(client aws.S3Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		buckets, err := client.ListBuckets(ctx)
		return bucketsResultMsg{buckets: buckets, err: err}
	}
}

func downloadObject(client aws.S3Client, bucket, key string) tea.Cmd {
	return func() tea.Msg {
		localPath := filepath.Base(key)
		f, err := os.Create(localPath)
		if err != nil {
			return downloadCompleteMsg{key: key, err: fmt.Errorf("creating %s: %w", localPath, err)}
		}
		defer f.Close()
		ctx := context.Background()
		if err := client.Download(ctx, bucket, key, f, nil); err != nil {
			os.Remove(localPath)
			return downloadCompleteMsg{key: key, err: err}
		}
		return downloadCompleteMsg{key: key, localPath: localPath}
	}
}

func restoreObject(client aws.S3Client, bucket, key string, days int32, tier string) tea.Cmd {
	return func() tea.Msg {
		parsedTier, err := aws.ParseTier(tier)
		if err != nil {
			return restoreCompleteMsg{key: key, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = client.RestoreObject(ctx, aws.RestoreInput{
			Bucket: bucket, Key: key, Days: days, Tier: parsedTier,
		})
		return restoreCompleteMsg{key: key, err: err}
	}
}

func loadProfiles() tea.Cmd {
	return func() tea.Msg {
		profiles, err := readAWSProfiles()
		return profilesResultMsg{profiles: profiles, err: err}
	}
}

func testProfile(bucketName, profile string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := aws.NewClient(ctx, "", profile)
		if err != nil {
			return profileTestMsg{profile: profile, bucket: bucketName, ok: false, err: err}
		}
		_, err = client.ListBuckets(ctx)
		return profileTestMsg{profile: profile, bucket: bucketName, ok: err == nil, err: err}
	}
}

// saveAWSProfile writes credentials to ~/.aws/credentials and region to ~/.aws/config.
func saveAWSProfile(profile, accessKey, secretKey, region string) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return profileSavedMsg{profile: profile, err: err}
		}
		awsDir := filepath.Join(home, ".aws")
		if err := os.MkdirAll(awsDir, 0o700); err != nil {
			return profileSavedMsg{profile: profile, err: fmt.Errorf("creating ~/.aws: %w", err)}
		}

		// Write credentials
		credSection := profile
		credValues := map[string]string{
			"aws_access_key_id": accessKey,
		}
		if secretKey != "" {
			credValues["aws_secret_access_key"] = secretKey
		}
		if err := updateINISection(filepath.Join(awsDir, "credentials"), credSection, credValues); err != nil {
			return profileSavedMsg{profile: profile, err: fmt.Errorf("writing credentials: %w", err)}
		}

		// Write config (region)
		if region != "" {
			configSection := profile
			if profile != "default" {
				configSection = "profile " + profile
			}
			configValues := map[string]string{"region": region}
			if err := updateINISection(filepath.Join(awsDir, "config"), configSection, configValues); err != nil {
				return profileSavedMsg{profile: profile, err: fmt.Errorf("writing config: %w", err)}
			}
		}

		return profileSavedMsg{profile: profile}
	}
}

// loadProfileDetail reads existing credentials for a profile to pre-fill the edit form.
func loadProfileDetail(profile string) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return profileDetailMsg{profile: profile, err: err}
		}
		accessKey := readINIValue(filepath.Join(home, ".aws", "credentials"), profile, "aws_access_key_id")

		// Region can be in [profile X] or [X] in config
		configSection := profile
		if profile != "default" {
			configSection = "profile " + profile
		}
		region := readINIValue(filepath.Join(home, ".aws", "config"), configSection, "region")

		return profileDetailMsg{profile: profile, accessKey: accessKey, region: region}
	}
}

// runAWSConfigureSSO suspends the TUI and shells out to `aws configure sso`.
func runAWSConfigureSSO(profile string) tea.Cmd {
	args := []string{"configure", "sso"}
	if profile != "" && profile != "default" {
		args = append(args, "--profile", profile)
	}
	c := exec.Command("aws", args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return awsConfigDoneMsg{err: err}
	})
}

// deleteAWSProfile removes a profile from ~/.aws/credentials and ~/.aws/config.
func deleteAWSProfile(profile string) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return profileSavedMsg{profile: profile, err: err}
		}
		awsDir := filepath.Join(home, ".aws")
		_ = removeINISection(filepath.Join(awsDir, "credentials"), profile)
		configSection := profile
		if profile != "default" {
			configSection = "profile " + profile
		}
		_ = removeINISection(filepath.Join(awsDir, "config"), configSection)
		return profileSavedMsg{profile: profile}
	}
}

// ---------------------------------------------------------------------------
// INI file helpers
// ---------------------------------------------------------------------------

// updateINISection merges key=value pairs into a [section], preserving existing keys not in values.
func updateINISection(path, section string, values map[string]string) error {
	content, _ := os.ReadFile(path)
	lines := strings.Split(string(content), "\n")

	// Find existing section boundaries
	start, end := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "["+section+"]" {
			start = i
			continue
		}
		if start >= 0 && end < 0 && strings.HasPrefix(trimmed, "[") {
			end = i
		}
	}
	if start >= 0 && end < 0 {
		end = len(lines)
		for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
	}

	if start >= 0 {
		// Merge: read existing keys, update with new values
		existing := map[string]string{}
		var keyOrder []string
		for _, line := range lines[start+1 : end] {
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				k := strings.TrimSpace(parts[0])
				existing[k] = strings.TrimSpace(parts[1])
				keyOrder = append(keyOrder, k)
			}
		}
		for k, v := range values {
			if _, ok := existing[k]; !ok {
				keyOrder = append(keyOrder, k)
			}
			existing[k] = v
		}

		var sectionLines []string
		sectionLines = append(sectionLines, "["+section+"]")
		for _, k := range keyOrder {
			sectionLines = append(sectionLines, k+" = "+existing[k])
		}

		newLines := make([]string, 0, len(lines))
		newLines = append(newLines, lines[:start]...)
		newLines = append(newLines, sectionLines...)
		newLines = append(newLines, lines[end:]...)
		lines = newLines
	} else {
		// Append new section
		var sectionLines []string
		sectionLines = append(sectionLines, "["+section+"]")
		for k, v := range values {
			sectionLines = append(sectionLines, k+" = "+v)
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, sectionLines...)
		lines = append(lines, "")
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}

// removeINISection removes a [section] and its contents from an INI file.
func removeINISection(path, section string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")

	start, end := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "["+section+"]" {
			start = i
			continue
		}
		if start >= 0 && end < 0 && strings.HasPrefix(trimmed, "[") {
			end = i
		}
	}
	if start < 0 {
		return nil // section not found
	}
	if end < 0 {
		end = len(lines)
	}
	// Also remove blank line before section if present
	if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}

	newLines := append(lines[:start], lines[end:]...)
	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o600)
}

// readINIValue reads a single value from an INI file section.
func readINIValue(path, section, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	inSection := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := line[1 : len(line)-1]
			inSection = (name == section)
			continue
		}
		if inSection && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if strings.TrimSpace(parts[0]) == key {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func clearFlashAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearFlashMsg{} })
}

func readAWSProfiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var profiles []string

	for _, rel := range []string{".aws/credentials", ".aws/config"} {
		f, err := os.Open(filepath.Join(home, rel))
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				name := strings.TrimPrefix(line[1:len(line)-1], "profile ")
				if !seen[name] {
					seen[name] = true
					profiles = append(profiles, name)
				}
			}
		}
		f.Close()
	}
	return profiles, nil
}
