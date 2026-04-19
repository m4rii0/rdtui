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
	"github.com/m4rii0/rdtui/internal/debug"
	"github.com/m4rii0/rdtui/internal/realdebrid"
	"github.com/m4rii0/rdtui/internal/version"
	"github.com/m4rii0/rdtui/pkg/models"
)

const refreshInterval = 5 * time.Second
const downloadRefreshInterval = time.Second

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
	modeOverwrite    mode = "overwrite"
	modeShowURL      mode = "show-url"
	modeDownload     mode = "download"
	modeSearch       mode = "search"
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
	handoffCopy     handoffAction = "copy"
	handoffDownload handoffAction = "download"
)

type batchOp string

const (
	batchOpDelete batchOp = "delete"
	batchOpCopy   batchOp = "copy"
)

type batchResultMsg struct {
	op      batchOp
	total   int
	success int
	failed  int
	skipped int
	detail  string
	err     error
}

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

	version     string
	mode        mode
	returnMode  mode
	downloadDir string
	width       int
	height      int
	loading     bool
	status      string
	errText     string
	session     *models.AuthSession

	torrents         []models.Torrent
	filteredTorrents []models.Torrent
	selectedIdx      int
	detail           *models.TorrentInfo
	sortColumn       int
	sortAscending    bool
	filterApplied    bool
	filterMatches    map[string][]int

	batchMode     bool
	batchSelected map[string]bool

	input       textinput.Model
	inputPrompt string
	inputAction inputAction
	searchInput textinput.Model

	deviceCode        *models.DeviceCode
	browser           fileBrowserState
	selector          selectFilesState
	targets           targetPickerState
	pendingDownload   *pendingDownloadState
	showURL           string
	download          *models.ManagedDownload
	downloadTorrentID string
	deleteIDs         []string
}

type pendingDownloadState struct {
	URL           string
	Filename      string
	Path          string
	RemoteBytes   int64
	ExistingBytes int64
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

type managedDownloadMsg struct {
	result models.ManagedDownloadStart
	err    error
}

type resolvedDownloadMsg struct {
	url      string
	filename string
	filesize int64
	err      error
}

type managedDownloadStatusMsg struct {
	download models.ManagedDownload
	ok       bool
	err      error
}

type downloadPathMsg struct {
	action string
	err    error
}

type tickMsg time.Time

type devicePollTickMsg time.Time

type downloadTickMsg time.Time

func NewModel(service *app.Service) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Width = 64
	si := textinput.New()
	si.Prompt = "/"
	si.Placeholder = "search..."
	si.Width = 40
	return Model{
		service:       service,
		version:       version.Version,
		mode:          modeStarting,
		returnMode:    modeMain,
		downloadDir:   modelDownloadDir(service),
		input:         ti,
		searchInput:   si,
		sortColumn:    colAdded,
		sortAscending: false,
		batchSelected: map[string]bool{},
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
		debug.Log("Update: WindowSize %dx%d", msg.Width, msg.Height)
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
		if m.mode == modeMain || m.mode == modeDetail || m.mode == modeSearch {
			return m, tea.Batch(refreshCmd(m.service), tickCmd())
		}
		return m, tickCmd()

	case devicePollTickMsg:
		if m.deviceCode != nil && m.mode == modeDeviceAuth {
			return m, pollDeviceCmd(m.service, *m.deviceCode)
		}
		return m, nil

	case downloadTickMsg:
		if m.mode != modeDownload || m.download == nil || m.download.IsTerminal() {
			return m, nil
		}
		return m, tea.Batch(downloadStatusCmd(m.service), downloadTickCmd())

	case torrentsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		selectedID := m.selectedTorrentID()
		m.torrents = append([]models.Torrent(nil), msg.torrents...)
		sortTorrents(m.torrents, m.sortColumn, m.sortAscending)
		if m.filterApplied || m.mode == modeSearch {
			m.applyFilter()
		}
		m.selectedIdx = selectedIndexForID(m.visibleTorrents(), selectedID)
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
		if m.returnMode == modeDownload {
			m.downloadTorrentID = ""
		}
		m.mode = modeMain
		m.status = "Deleted torrent"
		m.errText = ""
		m.deleteIDs = nil
		return m, refreshCmd(m.service)

	case batchResultMsg:
		m.loading = false
		switch msg.op {
		case batchOpDelete:
			if msg.err != nil {
				m.errText = msg.err.Error()
				return m, nil
			}
			m.mode = modeMain
			if msg.failed > 0 {
				m.status = fmt.Sprintf("Deleted %d/%d torrent(s)", msg.success, msg.total)
			} else {
				m.status = fmt.Sprintf("Deleted %d torrent(s)", msg.success)
			}
			if msg.detail != "" {
				m.errText = strings.TrimSpace(msg.detail)
			} else {
				m.errText = ""
			}
			m.deleteIDs = nil
			m.clearBatchSelection()
			m.batchMode = false
			return m, refreshCmd(m.service)
		case batchOpCopy:
			if msg.err != nil {
				m.errText = msg.err.Error()
				return m, nil
			}
			m.mode = modeMain
			var parts []string
			if msg.success > 0 {
				parts = append(parts, fmt.Sprintf("Copied %d URL(s)", msg.success))
			}
			if msg.skipped > 0 {
				parts = append(parts, fmt.Sprintf("%d skipped (not downloaded)", msg.skipped))
			}
			if msg.failed > 0 {
				parts = append(parts, fmt.Sprintf("%d failed", msg.failed))
			}
			m.status = strings.Join(parts, ", ")
			if msg.detail != "" {
				m.errText = strings.TrimSpace(msg.detail)
			} else {
				m.errText = ""
			}
			m.clearBatchSelection()
			m.batchMode = false
			return m, nil
		}

	case handoffMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.showURL = msg.result.URL
		m.mode = modeShowURL
		if msg.result.Copied {
			m.status = "Direct URL copied to clipboard"
		} else {
			m.status = "Direct URL ready"
		}
		m.errText = ""
		return m, nil

	case managedDownloadMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		download := msg.result.Download
		m.download = &download
		m.mode = modeDownload
		m.errText = ""
		if msg.result.Reused {
			m.status = "Reopened active download: " + download.Filename
		} else {
			m.status = "Started download: " + download.Filename
		}
		if download.IsTerminal() {
			return m, nil
		}
		return m, tea.Batch(downloadStatusCmd(m.service), downloadTickCmd())

	case resolvedDownloadMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		path := filepath.Join(m.downloadDir, msg.filename)
		existingBytes, exists, err := existingFileSize(path)
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		if exists {
			m.pendingDownload = &pendingDownloadState{
				URL:           msg.url,
				Filename:      msg.filename,
				Path:          path,
				RemoteBytes:   msg.filesize,
				ExistingBytes: existingBytes,
			}
			m.mode = modeOverwrite
			m.errText = ""
			m.status = "Existing file found"
			return m, nil
		}
		return m.startPendingDownload(msg.url, msg.filename)

	case managedDownloadStatusMsg:
		m.loading = false
		if !msg.ok {
			return m, nil
		}
		download := msg.download
		m.download = &download
		if msg.err != nil {
			m.status = "Managed download failed"
			m.errText = ""
			return m, nil
		}
		if download.IsComplete() {
			m.status = "Download complete"
			m.errText = ""
			return m, nil
		}
		if download.IsError() {
			m.status = "Managed download failed"
			m.errText = ""
			return m, nil
		}
		return m, nil

	case downloadPathMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.errText = ""
		if msg.action == "open" {
			m.status = "Opened downloaded file"
		} else {
			m.status = "Opened download directory"
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	if m.needsTextInputUpdate() {
		m.input, cmd = m.input.Update(msg)
	} else if m.mode == modeSearch {
		m.searchInput, cmd = m.searchInput.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	debug.Log("handleKey: key=%q mode=%s", msg.String(), m.mode)
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
			if m.filterApplied {
				m.filterApplied = false
				m.filteredTorrents = nil
				m.selectedIdx = 0
				return m, nil
			}
			return m, tea.Quit
		case "/":
			if !m.filterApplied {
				m.searchInput.Reset()
			}
			m.mode = modeSearch
			m.errText = ""
			m.applyFilter()
			return m, m.searchInput.Focus()
		case "enter":
			vis := m.visibleTorrents()
			if len(vis) == 0 {
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
			vis := m.visibleTorrents()
			if m.selectedIdx < len(vis)-1 {
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
			m.browser = newFileBrowser(".")
			m.mode = modeFileBrowser
			m.errText = ""
			debug.Log("handler: entering file browser, dir=%s entries=%d", m.browser.CurrentDir, len(m.browser.Entries))
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
			if m.hasBatchSelection() {
				ids := m.batchSelectedIDs()
				m.loading = true
				m.returnMode = modeMain
				return m, batchOpCmd(m.service, batchOpCopy, ids, m.torrents)
			}
			m.returnMode = modeMain
			return m.beginHandoff(handoffCopy)
		case "d":
			if m.batchMode {
				return m, nil
			}
			m.returnMode = modeMain
			return m.beginHandoff(handoffDownload)
		case "b":
			m.batchMode = !m.batchMode
			if !m.batchMode {
				m.clearBatchSelection()
			}
			m.errText = ""
			return m, nil
		case " ":
			if m.batchMode && len(m.visibleTorrents()) > 0 {
				m.toggleBatchMark(m.selectedTorrentID())
				return m, nil
			}
		case "ctrl+a":
			if m.batchMode {
				m.selectAllTorrents()
				return m, nil
			}
		case "ctrl+d":
			if m.batchMode {
				m.clearBatchSelection()
				return m, nil
			}
		case "esc":
			if m.batchMode {
				m.clearBatchSelection()
				m.batchMode = false
				return m, nil
			}
		case "x":
			if m.hasBatchSelection() {
				m.deleteIDs = m.batchSelectedIDs()
				m.returnMode = modeMain
				m.mode = modeDelete
				return m, nil
			}
			if m.detail == nil {
				return m, nil
			}
			m.returnMode = modeMain
			m.mode = modeDelete
			m.deleteIDs = []string{m.detail.ID}
			return m, nil
		}

	case modeSearch:
		switch msg.String() {
		case "esc":
			m.searchInput.Blur()
			m.searchInput.Reset()
			m.filterApplied = false
			m.filteredTorrents = nil
			m.selectedIdx = 0
			m.mode = modeMain
			return m, nil
		case "enter":
			if m.searchInput.Value() == "" {
				m.filterApplied = false
				m.filteredTorrents = nil
			} else {
				m.filterApplied = true
				m.searchInput.Blur()
			}
			m.mode = modeMain
			return m, nil
		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				return m, detailCmd(m.service, m.selectedTorrentID())
			}
			return m, nil
		case "down", "j":
			vis := m.visibleTorrents()
			if m.selectedIdx < len(vis)-1 {
				m.selectedIdx++
				return m, detailCmd(m.service, m.selectedTorrentID())
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilter()
		return m, cmd

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
		case "d":
			m.returnMode = modeDetail
			return m.beginHandoff(handoffDownload)
		case "x":
			if m.detail == nil {
				return m, nil
			}
			m.returnMode = modeDetail
			m.mode = modeDelete
			m.deleteIDs = []string{m.detail.ID}
			return m, nil
		case "r":
			m.loading = true
			return m, refreshCmd(m.service)
		}

	case modeFileBrowser:
		m.errText = ""
		key := msg.String()
		debug.Log("handler: fileBrowser key=%q cursor=%d entries=%d selected=%d visual=%v",
			key, m.browser.Cursor, len(m.browser.Entries), len(m.browser.Selected), m.browser.VisualMode)
		switch key {
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
		case "V":
			m.browser.toggleVisual()
		case "ctrl+a":
			m.browser.toggleAll()
		case "ctrl+d":
			m.browser.clearSelection()
		case "H":
			m.browser.ShowHidden = !m.browser.ShowHidden
			m.browser.reload()
		case "backspace", "left", "h":
			m.browser.CurrentDir = filepathDir(m.browser.CurrentDir)
			m.browser.VisualMode = false
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
			if m.targets.Action == handoffDownload && m.detail != nil {
				m.downloadTorrentID = m.detail.ID
			}
			m.loading = true
			if m.targets.Action == handoffDownload {
				return m, resolveDownloadCmd(m.service, m.targets.Items[m.targets.Cursor])
			}
			return m, handoffCmd(m.service, m.targets.Items[m.targets.Cursor])
		}
		return m, nil

	case modeOverwrite:
		switch msg.String() {
		case "esc", "n":
			m.mode = m.returnMode
			m.pendingDownload = nil
			m.status = "Download cancelled"
			m.errText = ""
			return m, nil
		case "y", "enter":
			if m.pendingDownload == nil {
				m.mode = m.returnMode
				return m, nil
			}
			return m.startPendingDownload(m.pendingDownload.URL, m.pendingDownload.Filename)
		}
		return m, nil

	case modeShowURL:
		switch msg.String() {
		case "esc", "enter":
			m.mode = m.returnMode
			return m, nil
		}

	case modeDownload:
		switch msg.String() {
		case "esc":
			m.mode = m.returnMode
			if m.download != nil && !m.download.IsTerminal() {
				m.status = "Download continues in background"
			}
			return m, nil
		case "r":
			m.loading = true
			return m, downloadStatusCmd(m.service)
		case "o":
			if m.download == nil || !m.download.IsComplete() || m.download.FilePath == "" {
				return m, nil
			}
			m.loading = true
			return m, openDownloadCmd(m.service, m.download.FilePath)
		case "s":
			if m.download == nil || !m.download.IsComplete() || m.download.FilePath == "" {
				return m, nil
			}
			m.loading = true
			return m, revealDownloadCmd(m.service, m.download.FilePath)
		case "x":
			if m.download == nil || !m.download.IsComplete() || m.downloadTorrentID == "" {
				return m, nil
			}
			m.returnMode = modeDownload
			m.mode = modeDelete
			m.deleteIDs = []string{m.downloadTorrentID}
			return m, nil
		}

	case modeDelete:
		switch msg.String() {
		case "esc", "n":
			m.mode = m.returnMode
			m.deleteIDs = nil
			return m, nil
		case "y", "enter":
			if len(m.deleteIDs) > 1 {
				m.loading = true
				ids := make([]string, len(m.deleteIDs))
				copy(ids, m.deleteIDs)
				return m, batchOpCmd(m.service, batchOpDelete, ids, m.torrents)
			}
			if len(m.deleteIDs) == 0 {
				m.mode = m.returnMode
				return m, nil
			}
			m.loading = true
			return m, deleteCmd(m.service, m.deleteIDs[0])
		}
	}

	return m, nil
}

func (m *Model) beginHandoff(action handoffAction) (tea.Model, tea.Cmd) {
	if m.detail == nil {
		m.errText = "No torrent selected"
		return *m, nil
	}
	if m.detail.Status != "downloaded" {
		m.errText = "Selected torrent is not ready to download"
		return *m, nil
	}
	targets := app.DownloadTargets(*m.detail)
	if len(targets) == 0 {
		m.errText = "No downloadable links available"
		return *m, nil
	}
	m.targets = targetPickerState{Items: targets, Action: action}
	m.mode = modeChooseTarget
	return *m, nil
}

func (m Model) View() string {
	return renderView(m)
}

func (m Model) selectedTorrentID() string {
	vis := m.visibleTorrents()
	if len(vis) == 0 || m.selectedIdx < 0 || m.selectedIdx >= len(vis) {
		return ""
	}
	return vis[m.selectedIdx].ID
}

func (m *Model) hasBatchSelection() bool {
	return len(m.batchSelected) > 0
}

func (m *Model) toggleBatchMark(id string) {
	if id == "" {
		return
	}
	if m.batchSelected[id] {
		delete(m.batchSelected, id)
	} else {
		m.batchSelected[id] = true
	}
}

func (m *Model) selectAllTorrents() {
	for _, t := range m.visibleTorrents() {
		m.batchSelected[t.ID] = true
	}
}

func (m *Model) clearBatchSelection() {
	for k := range m.batchSelected {
		delete(m.batchSelected, k)
	}
}

func (m *Model) batchSelectedIDs() []string {
	ids := make([]string, 0, len(m.batchSelected))
	for _, t := range m.visibleTorrents() {
		if m.batchSelected[t.ID] {
			ids = append(ids, t.ID)
		}
	}
	return ids
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
	if m.filterApplied || m.mode == modeSearch {
		m.applyFilter()
	}
	m.selectedIdx = selectedIndexForID(m.visibleTorrents(), selectedID)
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

const batchDelay = 250 * time.Millisecond

func batchTick(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(batchDelay):
		return nil
	}
}

func batchOpCmd(service *app.Service, op batchOp, ids []string, torrents []models.Torrent) tea.Cmd {
	return func() tea.Msg {
		switch op {
		case batchOpDelete:
			return executeBatchDelete(service, ids)
		case batchOpCopy:
			return executeBatchCopy(service, ids, torrents)
		}
		return batchResultMsg{op: op, err: fmt.Errorf("unknown operation: %s", op)}
	}
}

func executeBatchDelete(service *app.Service, ids []string) batchResultMsg {
	result := batchResultMsg{op: batchOpDelete, total: len(ids)}
	for i, id := range ids {
		if i > 0 {
			if err := batchTick(context.Background()); err != nil {
				result.failed = len(ids) - i
				result.err = err
				return result
			}
		}
		if err := service.DeleteTorrent(context.Background(), id); err != nil {
			result.failed++
			result.detail += err.Error() + "\n"
		} else {
			result.success++
		}
	}
	return result
}

func executeBatchCopy(service *app.Service, ids []string, torrents []models.Torrent) batchResultMsg {
	result := batchResultMsg{op: batchOpCopy, total: len(ids)}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var urls []string
	first := true
	for _, t := range torrents {
		if !idSet[t.ID] {
			continue
		}
		if t.Status != "downloaded" {
			result.skipped++
			continue
		}
		if len(t.Links) == 0 {
			result.skipped++
			continue
		}
		for _, link := range t.Links {
			if !first {
				if err := batchTick(context.Background()); err != nil {
					result.failed = result.total - result.success - result.skipped
					result.err = err
					return result
				}
			}
			first = false
			unrestricted, err := service.ResolveDirectURL(context.Background(), models.DownloadTarget{Link: link})
			if err != nil {
				result.failed++
				result.detail += err.Error() + "\n"
				continue
			}
			u := unrestricted.Download
			if u == "" {
				u = unrestricted.Link
			}
			if u != "" {
				urls = append(urls, u)
			}
		}
		result.success++
	}
	if len(urls) > 0 {
		copied, err := service.CopyURL(strings.Join(urls, "\n"))
		if err != nil {
			result.err = err
			return result
		}
		if !copied {
			result.detail = "clipboard not available"
		}
	}
	return result
}

func handoffCmd(service *app.Service, target models.DownloadTarget) tea.Cmd {
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
		copied, err := service.CopyURL(url)
		return handoffMsg{result: models.HandoffResult{URL: url, Copied: copied}, err: err}
	}
}

func resolveDownloadCmd(service *app.Service, target models.DownloadTarget) tea.Cmd {
	return func() tea.Msg {
		unrestricted, err := service.ResolveDirectURL(context.Background(), target)
		if err != nil {
			return resolvedDownloadMsg{err: err}
		}
		url := unrestricted.Download
		if url == "" {
			url = unrestricted.Link
		}
		filename := unrestricted.Filename
		if filename == "" {
			filename = target.Label
		}
		return resolvedDownloadMsg{url: url, filename: filename, filesize: unrestricted.Filesize}
	}
}

func startManagedDownloadCmd(service *app.Service, url string, filename string) tea.Cmd {
	return func() tea.Msg {
		result, err := service.StartManagedDownload(context.Background(), url, filename)
		return managedDownloadMsg{result: result, err: err}
	}
}

func downloadTickCmd() tea.Cmd {
	return tea.Tick(downloadRefreshInterval, func(_ time.Time) tea.Msg {
		return downloadTickMsg(time.Now())
	})
}

func downloadStatusCmd(service *app.Service) tea.Cmd {
	return func() tea.Msg {
		download, ok, err := service.ManagedDownloadStatus(context.Background())
		return managedDownloadStatusMsg{download: download, ok: ok, err: err}
	}
}

func openDownloadCmd(service *app.Service, path string) tea.Cmd {
	return func() tea.Msg {
		return downloadPathMsg{action: "open", err: service.OpenFile(path)}
	}
}

func revealDownloadCmd(service *app.Service, path string) tea.Cmd {
	return func() tea.Msg {
		return downloadPathMsg{action: "reveal", err: service.RevealInDirectory(path)}
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

func modelDownloadDir(service *app.Service) string {
	if service != nil {
		if dir := service.Config().DefaultDownloadDir; dir != "" {
			return dir
		}
	}
	return filepath.Join(userHomeDir(), "Downloads")
}

func (m *Model) startPendingDownload(url string, filename string) (tea.Model, tea.Cmd) {
	m.pendingDownload = nil
	m.loading = true
	return *m, startManagedDownloadCmd(m.service, url, filename)
}

func existingFileSize(path string) (int64, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if info.IsDir() {
		return 0, false, fmt.Errorf("download target is directory: %s", path)
	}
	return info.Size(), true, nil
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
