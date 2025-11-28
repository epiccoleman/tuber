package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Download options (can be combined)
type DownloadOptions struct {
	Video   bool
	Audio   bool
	Subs    bool
	Summary bool
	Prompt  string
}

func (d DownloadOptions) String() string {
	var parts []string
	if d.Video {
		parts = append(parts, "Video")
	}
	if d.Audio {
		parts = append(parts, "Audio")
	}
	if d.Subs {
		parts = append(parts, "Subtitles")
	}
	if d.Summary {
		parts = append(parts, "Summary")
	}
	if len(parts) == 0 {
		return "Nothing"
	}
	return strings.Join(parts, " + ")
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
	url          string
	choices      []string
	cursor       int
	checked      []bool // which options are checked
	done         bool
	quitting     bool
	title        string
	outPath      string // full output path (dir + basename)
	editing      bool
	editBuf      string
	state        uiState
	editingField string // "path" or "prompt"
	prompt       string // custom summary prompt
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

	summaryLabel := "Summary"
	if !claudeAvailable {
		summaryLabel = "Summary (install claude cli)"
	}

	return model{
		url:     url,
		choices: []string{"Video", "Audio", "Subtitles", summaryLabel},
		checked: make([]bool, 4),
		state:   state,
		outPath: dir + "/video", // fallback
		prompt:  defaultPrompt,
	}
}

const defaultPrompt = "Summarize this transcript of a YouTube video. Provide a concise summary of the main points and key takeaways."

func (m model) getOptions() DownloadOptions {
	return DownloadOptions{
		Video:   m.checked[0],
		Audio:   m.checked[1],
		Subs:    m.checked[2],
		Summary: m.checked[3],
		Prompt:  m.prompt,
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
				if m.editingField == "path" {
					m.outPath = m.editBuf
				} else if m.editingField == "prompt" {
					m.prompt = m.editBuf
				}
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
		case " ", "x":
			// Toggle checkbox (but not Summary if claude unavailable)
			if m.cursor == 3 && !claudeAvailable {
				// Can't toggle summary without claude
				break
			}
			m.checked[m.cursor] = !m.checked[m.cursor]
		case "enter":
			// Submit if at least one option selected
			opts := m.getOptions()
			if opts.Video || opts.Audio || opts.Subs || opts.Summary {
				m.done = true
				return m, tea.Quit
			}
		case "e":
			m.editing = true
			m.editingField = "path"
			m.editBuf = m.outPath
		case "p":
			// Only allow prompt editing if claude is available
			if claudeAvailable {
				m.editing = true
				m.editingField = "prompt"
				m.editBuf = m.prompt
			}
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

		checkbox := "[ ]"
		if m.checked[i] {
			checkbox = "[x]"
		}

		s += cursor + checkbox + " " + style.Render(choice) + "\n"
	}

	// Show filename preview and prompt
	s += "\n"
	if m.editing {
		if m.editingField == "path" {
			s += editStyle.Render("Output: ") + m.editBuf + editStyle.Render("▌") + "\n"
		} else {
			s += editStyle.Render("Prompt: ") + m.editBuf + editStyle.Render("▌") + "\n"
		}
		s += dimStyle.Render("enter to confirm • esc to cancel")
	} else {
		s += dimStyle.Render("Output: ") + filenameStyle.Render(m.getFilenames()) + "\n"
		if claudeAvailable && m.checked[3] {
			// Show truncated prompt if summary is selected
			promptPreview := m.prompt
			if len(promptPreview) > 50 {
				promptPreview = promptPreview[:47] + "..."
			}
			s += dimStyle.Render("Prompt: ") + promptPreview + "\n"
		}
		hints := "↑/↓ navigate • space toggle • enter download • e edit path"
		if claudeAvailable {
			hints += " • p edit prompt"
		}
		hints += " • q quit"
		s += "\n" + dimStyle.Render(hints)
	}
	return s
}

func (m model) getFilenames() string {
	opts := m.getOptions()
	var exts []string

	if opts.Video {
		exts = append(exts, ".mp4")
	}
	if opts.Audio {
		exts = append(exts, ".mp3")
	}
	if opts.Subs {
		exts = append(exts, ".txt")
	}
	if opts.Summary {
		exts = append(exts, "(summary to stdout)")
	}

	if len(exts) == 0 {
		return "(select at least one option)"
	}

	// If only summary, no file output
	if len(exts) == 1 && opts.Summary {
		return exts[0]
	}

	// Build filename string
	var fileExts []string
	for _, e := range exts {
		if !strings.HasPrefix(e, "(") {
			fileExts = append(fileExts, e)
		}
	}

	result := ""
	if len(fileExts) > 0 {
		if len(fileExts) == 1 {
			result = m.outPath + fileExts[0]
		} else {
			result = m.outPath + ".{" + strings.Join(fileExts, ",")[1:] // strip leading dots, rejoin
		}
	}

	if opts.Summary {
		if result != "" {
			result += " + summary"
		} else {
			result = "(summary to stdout)"
		}
	}

	return result
}

func runDownload(url string, opts DownloadOptions) error {
	// Run file downloads with spinner
	if opts.Video || opts.Audio || opts.Subs {
		if err := runWithSpinner(url, opts); err != nil {
			return err
		}
	}

	// Summary runs separately (has its own output)
	if opts.Summary {
		prompt := opts.Prompt
		if prompt == "" {
			prompt = defaultPrompt
		}
		return downloadSummary(url, prompt)
	}

	return nil
}

// Spinner model for download progress
type downloadModel struct {
	spinner spinner.Model
	status  string
	done    bool
	err     error
	url     string
	opts    DownloadOptions
	steps   []string // what to download, in order
	step    int      // current step index
}

type downloadDoneMsg struct{ err error }

func initialDownloadModel(url string, opts DownloadOptions) downloadModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	// Build list of steps based on options
	var steps []string
	if opts.Video {
		steps = append(steps, "video")
	}
	if opts.Audio {
		steps = append(steps, "audio")
	}
	if opts.Subs {
		steps = append(steps, "subs")
	}

	dm := downloadModel{
		spinner: s,
		url:     url,
		opts:    opts,
		steps:   steps,
		step:    0,
	}
	dm.status = dm.getStatusText()

	return dm
}

func (m downloadModel) getStatusText() string {
	if m.step >= len(m.steps) {
		return "Done!"
	}

	stepName := m.steps[m.step]
	total := len(m.steps)
	current := m.step + 1

	var desc string
	switch stepName {
	case "video":
		desc = "Downloading video"
	case "audio":
		desc = "Downloading audio"
	case "subs":
		desc = "Downloading subtitles"
	}

	if total > 1 {
		return fmt.Sprintf("%s (%d/%d)...", desc, current, total)
	}
	return desc + "..."
}

type startDownloadMsg struct{}

func (m downloadModel) Init() tea.Cmd {
	// Start spinner first, then trigger download on next tick
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		return startDownloadMsg{}
	})
}

func (m downloadModel) runCurrentStep() tea.Cmd {
	return func() tea.Msg {
		if m.step >= len(m.steps) {
			return downloadDoneMsg{err: nil}
		}

		var err error
		switch m.steps[m.step] {
		case "video":
			err = doDownloadVideo(m.url)
		case "audio":
			err = doDownloadAudio(m.url)
		case "subs":
			err = doDownloadSubs(m.url)
		}
		return downloadDoneMsg{err: err}
	}
}

func (m downloadModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case startDownloadMsg:
		return m, m.runCurrentStep()

	case downloadDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}

		// Advance to next step
		m.step++
		if m.step < len(m.steps) {
			m.status = m.getStatusText()
			return m, m.runCurrentStep()
		}

		m.done = true
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m downloadModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.status
}

func runWithSpinner(url string, opts DownloadOptions) error {
	p := tea.NewProgram(initialDownloadModel(url, opts), tea.WithOutput(os.Stderr))
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	dm := finalModel.(downloadModel)
	return dm.err
}

var outputDir string
var customOutPath string

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

func doDownloadVideo(url string) error {
	args := []string{
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--merge-output-format", "mp4",
		"-q", "--no-warnings",
	}
	args = append(args, "-o", getOutputPattern(".%(ext)s"))
	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	return cmd.Run()
}

func doDownloadAudio(url string) error {
	args := []string{
		"-x",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"-q", "--no-warnings",
	}
	args = append(args, "-o", getOutputPattern(".%(ext)s"))
	args = append(args, url)
	cmd := exec.Command("yt-dlp", args...)
	return cmd.Run()
}

func doDownloadSubs(url string) error {
	cmd := exec.Command("yt-dlp",
		"--write-subs",
		"--write-auto-subs",
		"--sub-lang", "en",
		"--sub-format", "vtt",
		"--skip-download",
		"-q", "--no-warnings",
		"-o", getOutputPattern(".%(ext)s"),
		url,
	)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Find and process the vtt file
	searchDir := "."
	if outputDir != "" {
		searchDir = outputDir
	}
	if customOutPath != "" {
		// Extract directory from custom path
		lastSlash := strings.LastIndex(customOutPath, "/")
		if lastSlash > 0 {
			searchDir = customOutPath[:lastSlash]
		}
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
				continue
			}
			// Remove the original vtt file
			os.Remove(vttPath)
		}
	}
	return nil
}

func downloadSummary(url string, prompt string) error {
	fmt.Fprintln(os.Stderr, "📝 Fetching subtitles for summary...")

	// Create temp dir for subtitle download
	tmpDir, err := os.MkdirTemp("", "tuber-summary-")
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
	cmd.Stdout = os.Stderr
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

	fmt.Fprintln(os.Stderr, "\n🤖 Generating summary...\n")

	// Pipe to claude - summary goes to stdout so it can be captured
	cmd = exec.Command("claude", "-p", prompt)
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

var claudeAvailable bool

func main() {
	// Check for yt-dlp
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		fmt.Fprintln(os.Stderr, "Error: yt-dlp not found in PATH")
		fmt.Fprintln(os.Stderr, "Install it from: https://github.com/yt-dlp/yt-dlp")
		os.Exit(1)
	}

	// Check for claude (optional)
	_, err := exec.LookPath("claude")
	claudeAvailable = err == nil

	// Flags for quick access (can be combined)
	videoFlag := flag.Bool("v", false, "Download video")
	audioFlag := flag.Bool("a", false, "Download audio (mp3)")
	subsFlag := flag.Bool("s", false, "Download subtitles (text)")
	sumFlag := flag.Bool("sum", false, "Summarize video using AI")
	promptFlag := flag.String("p", "", "Custom prompt for summary")
	outFlag := flag.String("o", "", "Output directory (default: current directory)")
	flag.Parse()

	outputDir = *outFlag

	args := flag.Args()
	var url string
	if len(args) >= 1 {
		url = args[0]
	}

	// Build options from flags
	prompt := defaultPrompt
	if *promptFlag != "" {
		prompt = *promptFlag
	}

	opts := DownloadOptions{
		Video:   *videoFlag,
		Audio:   *audioFlag,
		Subs:    *subsFlag,
		Summary: *sumFlag,
		Prompt:  prompt,
	}

	// Check if summary requested but claude not available
	if opts.Summary && !claudeAvailable {
		fmt.Fprintln(os.Stderr, "Error: -sum requires claude cli")
		fmt.Fprintln(os.Stderr, "Install it from: https://claude.ai/download")
		os.Exit(1)
	}

	flagSet := opts.Video || opts.Audio || opts.Subs || opts.Summary

	// If flag set but no URL, show usage
	if flagSet && url == "" {
		fmt.Println("Usage: tuber [flags] <url>")
		fmt.Println("\nFlags (can be combined):")
		fmt.Println("  -v             Download video")
		fmt.Println("  -a             Download audio (mp3)")
		fmt.Println("  -s             Download subtitles (text)")
		fmt.Println("  -sum           Summarize video using AI")
		fmt.Println("  -p <prompt>    Custom prompt for summary")
		fmt.Println("  -o <dir>       Output directory")
		fmt.Println("\nExamples:")
		fmt.Println("  tuber -a -s <url>                    Download audio and subtitles")
		fmt.Println("  tuber -sum -p \"List key points\" <url>  Summarize with custom prompt")
		fmt.Println("\nWithout flags, opens interactive menu.")
		os.Exit(1)
	}

	// If no flag (or no URL), show interactive menu
	if !flagSet {
		p := tea.NewProgram(initialModel(url))
		m, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		finalModel := m.(model)
		if finalModel.quitting {
			os.Exit(0)
		}
		opts = finalModel.getOptions()
		url = finalModel.url
		customOutPath = finalModel.outPath
	}

	fmt.Fprintf(os.Stderr, "\nDownloading %s from:\n%s\n\n", opts, url)

	if err := runDownload(url, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "\n✓ Done!")
}
