package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/download"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/internal/version"
	"github.com/m4rii0/rdtui/pkg/models"
)

const refreshInterval = 5 * time.Second

type mode string

const (
	modeStarting     mode = "starting"
	modeAuthChoice   mode = "auth-choice"
	modeTokenInput   mode = "token-input"
	modeDeviceAuth   mode = "device-auth"
	modeMain         mode = "main"
	modeDetail       mode = "detail"
	modeMagnetInput  mode = "magnet-input"
	modeURLInput     mode = "url-input"
	modeFileBrowser  mode = "file-browser"
	modeSelectFiles  mode = "select-files"
	modeChooseTarget mode = "choose-target"
	modeShowURL      mode = "show-url"
	modeDelete       mode = "delete"
)

type inputAction string

const (
	inputToken  inputAction = "token"
	inputMagnet inputAction = "magnet"
	inputURL    inputAction = "url"
)

type handoffAction string

const (
	handoffCopy   handoffAction = "copy"
	handoffLaunch handoffAction = "launch"
)

type selectFilesState struct {
	Cursor   int
	Files    []models.TorrentFile
	Selected map[int]bool
}

type targetPickerState struct {
	Cursor int
	Action handoffAction
	Items  []models.DownloadTarget
}

type Model struct {
	service *app.Service

	version    string
	mode       mode
	returnMode mode
	width      int
	height     int
	loading    bool
	status     string
	errText    string
	session    *models.AuthSession

	torrents      []models.Torrent
	selectedIdx   int
	detail        *models.TorrentInfo
	sortColumn    int
	sortAscending bool

	input       textinput.Model
	inputPrompt string
	inputAction inputAction

	deviceCode *models.DeviceCode
	browser    fileBrowserState
	selector   selectFilesState
	targets    targetPickerState
	showURL    string
	deleteID   string
}

type bootstrapMsg struct {
	session *models.AuthSession
	err     error
}

type authMsg struct {
	session *models.AuthSession
	err     error
}

type deviceStartMsg struct {
	code models.DeviceCode
	err  error
}

type devicePollMsg struct {
	session *models.AuthSession
	err     error
}

type torrentsMsg struct {
	torrents []models.Torrent
	err      error
}

type detailMsg struct {
	id   string
	info models.TorrentInfo
	err  error
}

type addTorrentMsg struct {
	result models.AddTorrentResult
	err    error
	label  string
}

type importMsg struct {
	results []models.ImportResult
}

type selectFilesMsg struct {
	err error
}

type deleteMsg struct {
	err error
}

type handoffMsg struct {
	result models.HandoffResult
	err    error
}

type tickMsg time.Time

type devicePollTickMsg time.Time

func NewModel(service *app.Service) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Width = 64
	return Model{
		service:       service,
		version:       version.Version,
		mode:          modeStarting,
		returnMode:    modeMain,
		input:         ti,
		sortColumn:    colAdded,
		sortAscending: false,
	}
}

func (m Model) Init() tea.Cmd {
	return bootstrapCmd(m.service)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case bootstrapMsg:
		m.loading = false
		if msg.err == nil {
			m.session = msg.session
			m.mode = modeMain
			m.status = "Authenticated as " + msg.session.User.Username
			m.errText = ""
			return m, tea.Batch(refreshCmd(m.service), tickCmd())
		}
		if errors.Is(msg.err, auth.ErrNoValidCredentials) {
			m.mode = modeAuthChoice
			m.status = "Choose an authentication method"
			m.errText = ""
			return m, nil
		}
		m.mode = modeAuthChoice
		m.errText = msg.err.Error()
		return m, nil

	case authMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.session = msg.session
		m.mode = modeMain
		m.input.Blur()
		m.input.Reset()
		m.status = "Authenticated as " + msg.session.User.Username
		m.errText = ""
		return m, tea.Batch(refreshCmd(m.service), tickCmd())

	case deviceStartMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.mode = modeAuthChoice
			return m, nil
		}
		m.deviceCode = &msg.code
		m.mode = modeDeviceAuth
		m.status = "Finish device authorization in your browser"
		m.errText = ""
		return m, devicePollTick(time.Duration(max(1, msg.code.Interval)) * time.Second)

	case devicePollMsg:
		if msg.err == nil {
			m.session = msg.session
			m.mode = modeMain
			m.deviceCode = nil
			m.status = "Authenticated as " + msg.session.User.Username
			m.errText = ""
			return m, tea.Batch(refreshCmd(m.service), tickCmd())
		}
		if errors.Is(msg.err, realdebrid.ErrAuthorizationPending) {
			if m.deviceCode != nil && time.Since(m.deviceCode.RequestedAt) < time.Duration(m.deviceCode.ExpiresIn)*time.Second {
				return m, devicePollTick(time.Duration(max(1, m.deviceCode.Interval)) * time.Second)
			}
			m.errText = "device code expired"
			m.mode = modeAuthChoice
			m.deviceCode = nil
			return m, nil
		}
		m.errText = msg.err.Error()
		m.mode = modeAuthChoice
		m.deviceCode = nil
		return m, nil

	case tickMsg:
		if m.mode == modeMain || m.mode == modeDetail {
			return m, tea.Batch(refreshCmd(m.service), tickCmd())
		}
		return m, tickCmd()

	case devicePollTickMsg:
		if m.deviceCode != nil && m.mode == modeDeviceAuth {
			return m, pollDeviceCmd(m.service, *m.deviceCode)
		}
		return m, nil

	case torrentsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		selectedID := m.selectedTorrentID()
		m.torrents = append([]models.Torrent(nil), msg.torrents...)
		sortTorrents(m.torrents, m.sortColumn, m.sortAscending)
		m.selectedIdx = selectedIndexForID(m.torrents, selectedID)
		m.errText = ""
		if len(m.torrents) == 0 {
			m.detail = nil
			m.status = "No torrents found"
			return m, nil
		}
		return m, detailCmd(m.service, m.selectedTorrentID())

	case detailMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		if msg.id != m.selectedTorrentID() {
			return m, nil
		}
		m.detail = &msg.info
		m.errText = ""
		return m, nil

	case addTorrentMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.mode = modeMain
		m.status = fmt.Sprintf("Added %s (%s)", msg.label, msg.result.ID)
		m.errText = ""
		return m, refreshCmd(m.service)

	case importMsg:
		m.loading = false
		m.mode = modeMain
		success := 0
		failed := 0
		failedNames := []string{}
		for _, result := range msg.results {
			if result.OK() {
				success++
			} else {
				failed++
				failedNames = append(failedNames, filepathBase(result.Source))
			}
		}
		if failed == 0 {
			m.status = fmt.Sprintf("Imported %d torrent file(s)", success)
			m.errText = ""
		} else {
			m.status = fmt.Sprintf("Imported %d torrent file(s), %d failed", success, failed)
			m.errText = "Failed: " + strings.Join(failedNames, ", ")
		}
		return m, refreshCmd(m.service)

	case selectFilesMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.mode = m.returnMode
		m.status = "Updated torrent file selection"
		m.errText = ""
		return m, refreshCmd(m.service)

	case deleteMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.mode = modeMain
		m.status = "Deleted torrent"
		m.errText = ""
		m.deleteID = ""
		return m, refreshCmd(m.service)

	case handoffMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.showURL = msg.result.URL
		m.mode = modeShowURL
		if msg.result.Launched {
			m.status = "Downloader launched: " + strings.Join(msg.result.Command, " ")
		} else if msg.result.Copied {
			m.status = "Direct URL copied to clipboard"
		} else {
			m.status = "Direct URL ready"
		}
		m.errText = ""
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	if m.needsTextInputUpdate() {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case modeAuthChoice:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "1", "t":
			m.mode = modeTokenInput
			m.inputAction = inputToken
			m.inputPrompt = "Enter Real-Debrid private token"
			m.input.Reset()
			m.input.Placeholder = "Private API token"
			m.errText = ""
			return m, m.input.Focus()
		case "2", "d":
			m.loading = true
			m.errText = ""
			return m, startDeviceCmd(m.service)
		}

	case modeTokenInput, modeMagnetInput, modeURLInput:
		switch msg.String() {
		case "esc":
			m.mode = fallbackModeForInput(m.inputAction)
			m.input.Blur()
			m.input.Reset()
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				m.errText = "Input cannot be empty"
				return m, nil
			}
			m.loading = true
			switch m.inputAction {
			case inputToken:
				return m, authTokenCmd(m.service, value)
			case inputMagnet:
				return m, addMagnetCmd(m.service, value)
			case inputURL:
				return m, addURLCmd(m.service, value)
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case modeDeviceAuth:
		switch msg.String() {
		case "esc":
			m.mode = modeAuthChoice
			m.deviceCode = nil
			return m, nil
		case "enter", "r":
			if m.deviceCode != nil {
				return m, pollDeviceCmd(m.service, *m.deviceCode)
			}
		}

	case modeMain:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "enter":
			if len(m.torrents) == 0 {
				return m, nil
			}
			m.mode = modeDetail
			if m.detail == nil || m.detail.ID != m.selectedTorrentID() {
				m.loading = true
				return m, detailCmd(m.service, m.selectedTorrentID())
			}
			return m, nil
		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				return m, detailCmd(m.service, m.selectedTorrentID())
			}
		case "down", "j":
			if m.selectedIdx < len(m.torrents)-1 {
				m.selectedIdx++
				return m, detailCmd(m.service, m.selectedTorrentID())
			}
		case "S":
			m.sortByColumn(colStatus)
			return m, nil
		case "P":
			m.sortByColumn(colProgress)
			return m, nil
		case "Z":
			m.sortByColumn(colSize)
			return m, nil
		case "D":
			m.sortByColumn(colAdded)
			return m, nil
		case "N":
			m.sortByColumn(colName)
			return m, nil
		case "r":
			m.loading = true
			return m, refreshCmd(m.service)
		case "m":
			m.mode = modeMagnetInput
			m.inputAction = inputMagnet
			m.inputPrompt = "Paste a magnet link"
			m.input.Reset()
			m.input.Placeholder = "magnet:?xt=urn:btih:..."
			m.errText = ""
			return m, m.input.Focus()
		case "u":
			m.mode = modeURLInput
			m.inputAction = inputURL
			m.inputPrompt = "Paste a .torrent URL"
			m.input.Reset()
			m.input.Placeholder = "https://example.com/file.torrent"
			m.errText = ""
			return m, m.input.Focus()
		case "i":
			startDir := userHomeDir()
			m.browser = newFileBrowser(startDir)
			m.mode = modeFileBrowser
			m.errText = ""
			return m, nil
		case "s":
			if m.detail == nil || m.detail.Status != "waiting_files_selection" {
				m.errText = "Selected torrent is not waiting for file selection"
				return m, nil
			}
			m.returnMode = modeMain
			m.selector = newSelectFilesState(*m.detail)
			m.mode = modeSelectFiles
			return m, nil
		case "y":
			m.returnMode = modeMain
			return m.beginHandoff(handoffCopy)
		case "x":
			if m.detail == nil {
				return m, nil
			}
			m.returnMode = modeMain
			m.mode = modeDelete
			m.deleteID = m.detail.ID
			return m, nil
		}

	case modeDetail:
		switch msg.String() {
		case "esc":
			m.mode = modeMain
			return m, nil
		case "s":
			if m.detail == nil || m.detail.Status != "waiting_files_selection" {
				m.errText = "Selected torrent is not waiting for file selection"
				return m, nil
			}
			m.returnMode = modeDetail
			m.selector = newSelectFilesState(*m.detail)
			m.mode = modeSelectFiles
			return m, nil
		case "y":
			m.returnMode = modeDetail
			return m.beginHandoff(handoffCopy)
		case "x":
			if m.detail == nil {
				return m, nil
			}
			m.returnMode = modeDetail
			m.mode = modeDelete
			m.deleteID = m.detail.ID
			return m, nil
		case "r":
			m.loading = true
			return m, refreshCmd(m.service)
		}

	case modeFileBrowser:
		switch msg.String() {
		case "esc":
			m.mode = modeMain
			return m, nil
		case "up", "k":
			m.browser.move(-1)
		case "down", "j":
			m.browser.move(1)
		case "enter", "right", "l":
			m.browser.openCurrent()
		case " ":
			m.browser.toggleCurrent()
		case "backspace", "left", "h":
			m.browser.CurrentDir = filepathDir(m.browser.CurrentDir)
			m.browser.reload()
		case "ctrl+s":
			selected := m.browser.selectedPaths()
			valid, invalid := app.ValidTorrentFiles(selected)
			if len(valid) == 0 {
				if len(invalid) > 0 {
					m.errText = "No valid .torrent files selected"
				} else {
					m.errText = "Select at least one .torrent file"
				}
				return m, nil
			}
			m.loading = true
			return m, importCmd(m.service, valid)
		}
		return m, nil

	case modeSelectFiles:
		switch msg.String() {
		case "esc":
			m.mode = m.returnMode
			return m, nil
		case "up", "k":
			if m.selector.Cursor > 0 {
				m.selector.Cursor--
			}
		case "down", "j":
			if m.selector.Cursor < len(m.selector.Files)-1 {
				m.selector.Cursor++
			}
		case " ":
			m.selector.toggleCurrent()
		case "enter":
			ids := m.selector.selectedIDs()
			if len(ids) == 0 {
				m.errText = "Select at least one file"
				return m, nil
			}
			m.loading = true
			return m, selectFilesCmd(m.service, m.detail.ID, ids)
		}
		return m, nil

	case modeChooseTarget:
		switch msg.String() {
		case "esc":
			m.mode = m.returnMode
			return m, nil
		case "up", "k":
			if m.targets.Cursor > 0 {
				m.targets.Cursor--
			}
		case "down", "j":
			if m.targets.Cursor < len(m.targets.Items)-1 {
				m.targets.Cursor++
			}
		case "enter":
			if len(m.targets.Items) == 0 {
				return m, nil
			}
			m.loading = true
			return m, handoffCmd(m.service, m.targets.Items[m.targets.Cursor], m.targets.Action)
		}
		return m, nil

	case modeShowURL:
		switch msg.String() {
		case "esc", "enter":
			m.mode = m.returnMode
			return m, nil
		}

	case modeDelete:
		switch msg.String() {
		case "esc", "n":
			m.mode = m.returnMode
			m.deleteID = ""
			return m, nil
		case "y", "enter":
			m.loading = true
			return m, deleteCmd(m.service, m.deleteID)
		}
	}

	return m, nil
}

func (m *Model) beginHandoff(action handoffAction) (tea.Model, tea.Cmd) {
	if m.detail == nil {
		m.errText = "No torrent selected"
		return m, nil
	}
	if m.detail.Status != "downloaded" {
		m.errText = "Selected torrent is not ready for download handoff"
		return m, nil
	}
	targets := app.DownloadTargets(*m.detail)
	if len(targets) == 0 {
		m.errText = "No downloadable links available"
		return m, nil
	}
	if action == handoffLaunch && len(m.service.Config().ExternalCommand) == 0 {
		action = handoffCopy
		m.status = "No downloader configured; showing the direct URL instead"
	}
	m.targets = targetPickerState{Items: targets, Action: action}
	m.mode = modeChooseTarget
	return m, nil
}

func (m Model) View() string {
	return renderView(m)
}

func (m Model) selectedTorrentID() string {
	if len(m.torrents) == 0 || m.selectedIdx < 0 || m.selectedIdx >= len(m.torrents) {
		return ""
	}
	return m.torrents[m.selectedIdx].ID
}

func (m *Model) sortByColumn(col int) {
	selectedID := m.selectedTorrentID()
	if m.sortColumn == col {
		m.sortAscending = !m.sortAscending
	} else {
		m.sortColumn = col
		m.sortAscending = false
	}
	sortTorrents(m.torrents, m.sortColumn, m.sortAscending)
	m.selectedIdx = selectedIndexForID(m.torrents, selectedID)
	dir := "↓"
	if m.sortAscending {
		dir = "↑"
	}
	m.status = "Sorted by " + columnLabel(m.sortColumn) + " " + dir
	m.errText = ""
}

func (m Model) needsTextInputUpdate() bool {
	return m.mode == modeTokenInput || m.mode == modeMagnetInput || m.mode == modeURLInput
}

func newSelectFilesState(info models.TorrentInfo) selectFilesState {
	selected := map[int]bool{}
	for _, id := range app.DefaultFileSelection(info) {
		selected[id] = true
	}
	return selectFilesState{Files: info.Files, Selected: selected}
}

func (s *selectFilesState) toggleCurrent() {
	if s.Cursor < 0 || s.Cursor >= len(s.Files) {
		return
	}
	id := s.Files[s.Cursor].ID
	if s.Selected[id] {
		delete(s.Selected, id)
		return
	}
	s.Selected[id] = true
}

func (s selectFilesState) selectedIDs() []int {
	ids := make([]int, 0, len(s.Selected))
	for _, file := range s.Files {
		if s.Selected[file.ID] {
			ids = append(ids, file.ID)
		}
	}
	return ids
}

func bootstrapCmd(service *app.Service) tea.Cmd {
	return func() tea.Msg {
		session, err := service.Bootstrap(context.Background())
		return bootstrapMsg{session: session, err: err}
	}
}

func authTokenCmd(service *app.Service, token string) tea.Cmd {
	return func() tea.Msg {
		session, err := service.AuthenticateWithToken(context.Background(), token, true)
		return authMsg{session: session, err: err}
	}
}

func startDeviceCmd(service *app.Service) tea.Cmd {
	return func() tea.Msg {
		code, err := service.StartDeviceFlow(context.Background())
		return deviceStartMsg{code: code, err: err}
	}
}

func pollDeviceCmd(service *app.Service, code models.DeviceCode) tea.Cmd {
	return func() tea.Msg {
		session, err := service.CompleteDeviceFlow(context.Background(), code)
		return devicePollMsg{session: session, err: err}
	}
}

func devicePollTick(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(_ time.Time) tea.Msg {
		return devicePollTickMsg(time.Now())
	})
}

func refreshCmd(service *app.Service) tea.Cmd {
	return func() tea.Msg {
		torrents, err := service.ListTorrents(context.Background())
		return torrentsMsg{torrents: torrents, err: err}
	}
}

func detailCmd(service *app.Service, id string) tea.Cmd {
	if id == "" {
		return nil
	}
	return func() tea.Msg {
		info, err := service.TorrentInfo(context.Background(), id)
		return detailMsg{id: id, info: info, err: err}
	}
}

func addMagnetCmd(service *app.Service, magnet string) tea.Cmd {
	return func() tea.Msg {
		result, err := service.AddMagnet(context.Background(), magnet)
		return addTorrentMsg{result: result, err: err, label: "magnet"}
	}
}

func addURLCmd(service *app.Service, remoteURL string) tea.Cmd {
	return func() tea.Msg {
		result, err := service.AddTorrentURL(context.Background(), remoteURL)
		return addTorrentMsg{result: result, err: err, label: "remote torrent URL"}
	}
}

func importCmd(service *app.Service, paths []string) tea.Cmd {
	return func() tea.Msg {
		return importMsg{results: service.ImportTorrentFiles(context.Background(), paths)}
	}
}

func selectFilesCmd(service *app.Service, torrentID string, ids []int) tea.Cmd {
	return func() tea.Msg {
		return selectFilesMsg{err: service.SelectFiles(context.Background(), torrentID, ids)}
	}
}

func deleteCmd(service *app.Service, torrentID string) tea.Cmd {
	return func() tea.Msg {
		return deleteMsg{err: service.DeleteTorrent(context.Background(), torrentID)}
	}
}

func handoffCmd(service *app.Service, target models.DownloadTarget, action handoffAction) tea.Cmd {
	return func() tea.Msg {
		unrestricted, err := service.ResolveDirectURL(context.Background(), target)
		if err != nil {
			return handoffMsg{err: err}
		}
		url := unrestricted.Download
		if url == "" {
			url = unrestricted.Link
		}
		filename := unrestricted.Filename
		if filename == "" {
			filename = target.Label
		}
		if action == handoffLaunch {
			result, err := service.LaunchDownload(url, filename)
			if errors.Is(err, download.ErrNoCommandConfigured) {
				copied, copyErr := service.CopyURL(url)
				return handoffMsg{result: models.HandoffResult{URL: url, Copied: copied}, err: copyErr}
			}
			return handoffMsg{result: result, err: err}
		}
		copied, err := service.CopyURL(url)
		return handoffMsg{result: models.HandoffResult{URL: url, Copied: copied}, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func selectedIndexForID(torrents []models.Torrent, id string) int {
	if len(torrents) == 0 {
		return 0
	}
	for idx, torrent := range torrents {
		if torrent.ID == id {
			return idx
		}
	}
	return 0
}

func fallbackModeForInput(action inputAction) mode {
	if action == inputToken {
		return modeAuthChoice
	}
	return modeMain
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}

func filepathDir(path string) string {
	if path == "" {
		return "."
	}
	return filepath.Dir(path)
}

func filepathBase(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}
