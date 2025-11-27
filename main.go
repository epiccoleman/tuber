package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Download modes
type Mode int

const (
	ModeVideo Mode = iota
	ModeAudio
	ModeSubs
	ModeAll
	ModeSummary
)

func (m Mode) String() string {
	return [...]string{"Video", "Audio", "Subtitles", "All", "Summary"}[m]
}

// UI State
type uiState int

const (
	stateURLInput uiState = iota
	stateLoading
	stateMenu
)

// TUI Model
type model struct {
	url       string
	choices   []string
	cursor    int
	selected  Mode
	done      bool
	quitting  bool
	title     string
	outPath   string // full output path (dir + basename)
	editing   bool
	editBuf   string
	state     uiState
}

// Message types for async operations
type titleMsg string
type errMsg error

func fetchTitle(url string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("yt-dlp", "--get-title", url)
		out, err := cmd.Output()
		if err != nil {
			return errMsg(err)
		}
		return titleMsg(strings.TrimSpace(string(out)))
	}
}

func initialModel(url string) model {
	state := stateURLInput
	if url != "" {
		state = stateLoading
	}

	dir := "."
	if outputDir != "" {
		dir = outputDir
	}

	return model{
		url:     url,
		choices: []string{"Video (best quality)", "Audio (mp3)", "Subtitles (text)", "All of the above", "Summary (AI)"},
		state:   state,
		outPath: dir + "/video", // fallback
	}
}

func (m model) Init() tea.Cmd {
	if m.url != "" {
		return fetchTitle(m.url)
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case titleMsg:
		m.title = string(msg)
		dir := "."
		if outputDir != "" {
			dir = outputDir
		}
		m.outPath = dir + "/" + sanitizeFilename(m.title)
		m.state = stateMenu
		return m, nil

	case errMsg:
		m.state = stateMenu
		// Keep fallback outPath
		return m, nil

	case tea.KeyMsg:
		// Handle URL input state
		if m.state == stateURLInput {
			switch msg.Type {
			case tea.KeyCtrlC:
				m.quitting = true
				return m, tea.Quit
			case tea.KeyEnter:
				if m.url != "" {
					m.state = stateLoading
					return m, fetchTitle(m.url)
				}
			case tea.KeyBackspace:
				if len(m.url) > 0 {
					m.url = m.url[:len(m.url)-1]
				}
			case tea.KeyRunes:
				m.url += string(msg.Runes)
			}
			return m, nil
		}

		// Handle editing mode
		if m.editing {
			switch msg.Type {
			case tea.KeyEnter:
				m.outPath = m.editBuf
				m.editing = false
			case tea.KeyEscape:
				m.editing = false
			case tea.KeyBackspace:
				if len(m.editBuf) > 0 {
					m.editBuf = m.editBuf[:len(m.editBuf)-1]
				}
			case tea.KeyRunes:
				m.editBuf += string(msg.Runes)
			}
			return m, nil
		}

		// Menu mode
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.selected = Mode(m.cursor)
			m.done = true
			return m, tea.Quit
		case "e":
			m.editing = true
			m.editBuf = m.outPath
		}
	}
	return m, nil
}

func sanitizeFilename(s string) string {
	// Remove or replace characters that are problematic in filenames
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	return replacer.Replace(s)
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	filenameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("cyan"))

	editStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("yellow")).
			Bold(true)
)

func (m model) View() string {
	if m.quitting {
		return ""
	}

	// URL input state
	if m.state == stateURLInput {
		s := titleStyle.Render("Enter YouTube URL:") + "\n\n"
		s += filenameStyle.Render(m.url) + editStyle.Render("▌") + "\n\n"
		s += dimStyle.Render("enter to continue • ctrl+c to quit")
		return s
	}

	// Loading state
	if m.state == stateLoading {
		return dimStyle.Render("Fetching video info...")
	}

	// Menu state
	s := titleStyle.Render("What would you like to download?") + "\n\n"

	for i, choice := range m.choices {
		cursor := "  "
		style := normalStyle
		if m.cursor == i {
			cursor = "▸ "
			style = selectedStyle
		}
		s += cursor + style.Render(choice) + "\n"
	}

	// Show filename preview
	s += "\n"
	if m.editing {
		s += editStyle.Render("Output: ") + m.editBuf + editStyle.Render("▌") + "\n"
		s += dimStyle.Render("enter to confirm • esc to cancel")
	} else {
		s += dimStyle.Render("Output: ") + filenameStyle.Render(m.getFilenames()) + "\n"
		s += "\n" + dimStyle.Render("↑/↓ navigate • enter select • e edit path • q quit")
	}
	return s
}

func (m model) getFilenames() string {
	mode := Mode(m.cursor)
	switch mode {
	case ModeVideo:
		return m.outPath + ".mp4"
	case ModeAudio:
		return m.outPath + ".mp3"
	case ModeSubs:
		return m.outPath + ".txt"
	case ModeAll:
		return m.outPath + ".{mp4,mp3,txt}"
	case ModeSummary:
		return "(summary printed to stdout)"
	}
	return ""
}

func runDownload(url string, mode Mode) error {
	switch mode {
	case ModeVideo:
		return downloadVideo(url)
	case ModeAudio:
		return downloadAudio(url)
	case ModeSubs:
		return downloadSubs(url)
	case ModeAll:
		if err := downloadVideo(url); err != nil {
			return fmt.Errorf("video download failed: %w", err)
		}
		if err := downloadAudio(url); err != nil {
			return fmt.Errorf("audio download failed: %w", err)
		}
		if err := downloadSubs(url); err != nil {
			return fmt.Errorf("subtitle download failed: %w", err)
		}
		return nil
	case ModeSummary:
		return downloadSummary(url)
	}
	return nil
}

var outputDir string
var customOutPath string

func downloadVideo(url string) error {
	fmt.Println("📹 Downloading video...")
	args := []string{
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--merge-output-format", "mp4",
	}
	args = append(args, "-o", getOutputPattern(".%(ext)s"))
	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getOutputPattern(ext string) string {
	if customOutPath != "" {
		return customOutPath + ext
	}
	dir := "."
	if outputDir != "" {
		dir = outputDir
	}
	return dir + "/%(title)s" + ext
}

func downloadAudio(url string) error {
	fmt.Println("🎵 Downloading audio...")
	args := []string{
		"-x",
		"--audio-format", "mp3",
		"--audio-quality", "0",
	}
	args = append(args, "-o", getOutputPattern(".%(ext)s"))
	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func downloadSubs(url string) error {
	fmt.Println("📝 Downloading subtitles...")

	cmd := exec.Command("yt-dlp",
		"--write-subs",
		"--write-auto-subs",
		"--sub-lang", "en",
		"--sub-format", "vtt",
		"--skip-download",
		"-o", getOutputPattern(".%(ext)s"),
		url,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// Find and process the vtt file
	searchDir := "."
	if outputDir != "" {
		searchDir = outputDir
	}
	return processSubtitles(searchDir)
}

func processSubtitles(dir string) error {
	// Find .vtt files in directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".vtt") {
			vttPath := dir + "/" + entry.Name()
			txtPath := dir + "/" + strings.TrimSuffix(entry.Name(), ".vtt") + ".txt"
			if err := dedupeVTT(vttPath, txtPath); err != nil {
				fmt.Printf("Warning: could not process %s: %v\n", entry.Name(), err)
				continue
			}
			// Remove the original vtt file
			os.Remove(vttPath)
			fmt.Printf("✓ Created %s\n", txtPath)
		}
	}
	return nil
}

func downloadSummary(url string) error {
	fmt.Println("📝 Fetching subtitles for summary...")

	// Create temp dir for subtitle download
	tmpDir, err := os.MkdirTemp("", "ytd-summary-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download subs to temp dir
	cmd := exec.Command("yt-dlp",
		"--write-subs",
		"--write-auto-subs",
		"--sub-lang", "en",
		"--sub-format", "vtt",
		"--skip-download",
		"-o", tmpDir+"/%(title)s.%(ext)s",
		url,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download subtitles: %w", err)
	}

	// Find the vtt file
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}

	var vttPath string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".vtt") {
			vttPath = tmpDir + "/" + entry.Name()
			break
		}
	}

	if vttPath == "" {
		return fmt.Errorf("no subtitles found for this video")
	}

	// Extract and dedupe the text
	transcript, err := extractText(vttPath)
	if err != nil {
		return fmt.Errorf("failed to extract text: %w", err)
	}

	fmt.Println("\n🤖 Generating summary...\n")

	// Pipe to claude
	cmd = exec.Command("claude", "-p", "Summarize this transcript of a YouTube video. Provide a concise summary of the main points and key takeaways.")
	cmd.Stdin = strings.NewReader(transcript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// extractText returns deduplicated plain text from a VTT file
func extractText(vttPath string) (string, error) {
	content, err := os.ReadFile(vttPath)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	var textLines []string
	seen := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip VTT header, timestamps, and empty lines
		if line == "" || line == "WEBVTT" || line == "Kind: captions" ||
			strings.HasPrefix(line, "Language:") ||
			strings.Contains(line, "-->") ||
			strings.HasPrefix(line, "NOTE") ||
			isTimestamp(line) {
			continue
		}

		// Remove HTML-style tags
		line = stripTags(line)
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Dedupe
		if !seen[line] {
			seen[line] = true
			textLines = append(textLines, line)
		}
	}

	return strings.Join(textLines, "\n"), nil
}

func dedupeVTT(vttPath, txtPath string) error {
	content, err := os.ReadFile(vttPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var textLines []string
	seen := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip VTT header, timestamps, and empty lines
		if line == "" || line == "WEBVTT" || line == "Kind: captions" ||
			strings.HasPrefix(line, "Language:") ||
			strings.Contains(line, "-->") ||
			strings.HasPrefix(line, "NOTE") ||
			isTimestamp(line) {
			continue
		}

		// Remove HTML-style tags like <c>, </c>, etc.
		line = stripTags(line)
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Dedupe: only add if we haven't seen this exact line
		if !seen[line] {
			seen[line] = true
			textLines = append(textLines, line)
		}
	}

	output := strings.Join(textLines, "\n")
	return os.WriteFile(txtPath, []byte(output), 0644)
}

func isTimestamp(line string) bool {
	// Match patterns like "00:00:00.000" or position indicators
	return strings.Contains(line, ":") &&
		(strings.Contains(line, ".") || strings.Contains(line, ",")) &&
		len(line) < 30
}

func stripTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func main() {
	// Flags for quick access
	videoFlag := flag.Bool("v", false, "Download video only")
	audioFlag := flag.Bool("a", false, "Download audio only (mp3)")
	subsFlag := flag.Bool("s", false, "Download subtitles only (text)")
	allFlag := flag.Bool("all", false, "Download video, audio, and subtitles")
	sumFlag := flag.Bool("sum", false, "Summarize video using AI")
	outFlag := flag.String("o", "", "Output directory (default: current directory)")
	flag.Parse()

	outputDir = *outFlag

	args := flag.Args()
	var url string
	if len(args) >= 1 {
		url = args[0]
	}

	// Check which flag is set
	var mode Mode
	flagSet := false

	if *videoFlag {
		mode = ModeVideo
		flagSet = true
	} else if *audioFlag {
		mode = ModeAudio
		flagSet = true
	} else if *subsFlag {
		mode = ModeSubs
		flagSet = true
	} else if *allFlag {
		mode = ModeAll
		flagSet = true
	} else if *sumFlag {
		mode = ModeSummary
		flagSet = true
	}

	// If flag set but no URL, show usage
	if flagSet && url == "" {
		fmt.Println("Usage: ytd [flags] <url>")
		fmt.Println("\nFlags:")
		fmt.Println("  -v        Download video only")
		fmt.Println("  -a        Download audio only (mp3)")
		fmt.Println("  -s        Download subtitles only (text)")
		fmt.Println("  -all      Download everything")
		fmt.Println("  -sum      Summarize video using AI")
		fmt.Println("  -o <dir>  Output directory")
		fmt.Println("\nWithout flags, opens interactive menu.")
		os.Exit(1)
	}

	// If no flag (or no URL), show interactive menu
	if !flagSet || url == "" {
		p := tea.NewProgram(initialModel(url))
		m, err := p.Run()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		finalModel := m.(model)
		if finalModel.quitting {
			os.Exit(0)
		}
		mode = finalModel.selected
		url = finalModel.url
		customOutPath = finalModel.outPath
	}

	fmt.Printf("\nDownloading %s from:\n%s\n\n", mode, url)

	if err := runDownload(url, mode); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✓ Done!")
}
