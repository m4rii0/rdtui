package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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
	service app.AppService

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
	spinner     spinner.Model
	progress    progress.Model

	deviceCode        *models.DeviceCode
	browser           fileBrowserState
	selector          selectFilesState
	targets           targetPickerState
	pendingDownload   *pendingDownloadState
	showURL           string
	download          *models.ManagedDownload
	downloadTorrentID string
	deleteIDs         []string
	helpVisible       bool
	pendingAction     *pendingShortcutAction
	flash             flashState
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

func NewModel(service app.AppService) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.SetWidth(64)
	si := textinput.New()
	si.Prompt = "/"
	si.Placeholder = "search..."
	si.SetWidth(40)
	sp := spinner.New(
		spinner.WithSpinner(spinner.Meter),
		spinner.WithStyle(infoStyle),
	)
	pb := progress.New(
		progress.WithDefaultBlend(),
	)
	return Model{
		service:       service,
		version:       version.Version,
		mode:          modeStarting,
		returnMode:    modeMain,
		downloadDir:   modelDownloadDir(service),
		input:         ti,
		searchInput:   si,
		spinner:       sp,
		progress:      pb,
		sortColumn:    colAdded,
		sortAscending: false,
		batchSelected: map[string]bool{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(bootstrapCmd(m.service), m.spinner.Tick)
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

	case flashTimeoutMsg:
		m.clearFlash()
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
			m.clearPendingDetailAction(msg.id)
			m.errText = msg.err.Error()
			return m, nil
		}
		if msg.id != m.selectedTorrentID() {
			m.clearPendingDetailAction(msg.id)
			return m, nil
		}
		m.detail = &msg.info
		m.errText = ""
		if action, ok := m.consumePendingDetailAction(msg.id); ok {
			return m.handleShortcutAction(action)
		}
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
		m.errText = ""
		return m, tea.Batch(refreshCmd(m.service), m.setFlash(flashSuccess, "Updated torrent file selection"))

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
		m.errText = ""
		m.deleteIDs = nil
		return m, tea.Batch(refreshCmd(m.service), m.setFlash(flashSuccess, "Deleted torrent"))

	case batchResultMsg:
		m.loading = false
		switch msg.op {
		case batchOpDelete:
			if msg.err != nil {
				m.errText = msg.err.Error()
				return m, nil
			}
			m.mode = modeMain
			var flashMsg string
			if msg.failed > 0 {
				flashMsg = fmt.Sprintf("Deleted %d/%d torrent(s)", msg.success, msg.total)
			} else {
				flashMsg = fmt.Sprintf("Deleted %d torrent(s)", msg.success)
			}
			if msg.detail != "" {
				m.errText = strings.TrimSpace(msg.detail)
			} else {
				m.errText = ""
			}
			m.deleteIDs = nil
			m.clearBatchSelection()
			m.batchMode = false
			return m, tea.Batch(refreshCmd(m.service), m.setFlash(flashSuccess, flashMsg))
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
			flashMsg := strings.Join(parts, ", ")
			if msg.detail != "" {
				m.errText = strings.TrimSpace(msg.detail)
			} else {
				m.errText = ""
			}
			m.clearBatchSelection()
			m.batchMode = false
			return m, m.setFlash(flashSuccess, flashMsg)
		}

	case handoffMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.showURL = msg.result.URL
		m.mode = modeShowURL
		m.errText = ""
		flashMsg := "Direct URL ready"
		if msg.result.Copied {
			flashMsg = "Direct URL copied to clipboard"
		}
		return m, m.setFlash(flashSuccess, flashMsg)

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

	case browserReadDirMsg:
		m.browser.handleReadDir(msg)
		return m, nil

	case tea.KeyPressMsg:
		prevMode := m.mode
		model, cmd := m.handleKey(msg)
		if mm, ok := model.(Model); ok && mm.mode != prevMode {
			mm.clearFlash()
			return mm, cmd
		}
		return model, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	if m.needsTextInputUpdate() {
		m.input, cmd = m.input.Update(msg)
	} else if m.mode == modeSearch {
		m.searchInput, cmd = m.searchInput.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	debug.Log("handleKey: key=%q mode=%s", msg.String(), m.mode)
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.helpVisible {
		if action, ok := m.matchShortcut(msg); ok {
			return m.handleShortcutAction(action)
		}
		return m, nil
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
	}

	if action, ok := m.matchShortcut(msg); ok {
		return m.handleShortcutAction(action)
	}

	switch m.mode {
	case modeSearch:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilter()
		return m, cmd
	case modeFileBrowser:
		m.errText = ""
		debug.Log("handler: fileBrowser key=%q cursor=%d entries=%d selected=%d visual=%v editing=%v",
			msg.String(), m.browser.Cursor, len(m.browser.Entries), len(m.browser.Selected), m.browser.VisualMode, m.browser.EditingPath)
		if m.browser.EditingPath {
			var cmd tea.Cmd
			m.browser.pathInput, cmd = m.browser.pathInput.Update(msg)
			m.browser.updateCompletions()
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) handleShortcutAction(action shortcutAction) (tea.Model, tea.Cmd) {
	switch action {
	case actionToggleHelp:
		m.helpVisible = true
		return m, nil
	case actionCloseHelp:
		m.helpVisible = false
		return m, nil
	case actionQuit:
		if m.mode == modeMain && m.filterApplied {
			m.filterApplied = false
			m.filteredTorrents = nil
			m.selectedIdx = 0
			return m, nil
		}
		return m, tea.Quit
	case actionMoveUp:
		return m.handleMove(-1)
	case actionMoveDown:
		return m.handleMove(1)
	case actionOpenDetail:
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
	case actionOpenSearch:
		if !m.filterApplied {
			m.searchInput.Reset()
		}
		m.mode = modeSearch
		m.errText = ""
		m.applyFilter()
		return m, m.searchInput.Focus()
	case actionSortStatus:
		m.sortByColumn(colStatus)
		return m, nil
	case actionSortProgress:
		m.sortByColumn(colProgress)
		return m, nil
	case actionSortSize:
		m.sortByColumn(colSize)
		return m, nil
	case actionSortAdded:
		m.sortByColumn(colAdded)
		return m, nil
	case actionSortName:
		m.sortByColumn(colName)
		return m, nil
	case actionRefresh:
		switch m.mode {
		case modeDownload:
			m.loading = true
			return m, downloadStatusCmd(m.service)
		default:
			m.loading = true
			return m, refreshCmd(m.service)
		}
	case actionOpenMagnetInput:
		m.mode = modeMagnetInput
		m.inputAction = inputMagnet
		m.inputPrompt = "Paste a magnet link"
		m.input.Reset()
		m.input.Placeholder = "magnet:?xt=urn:btih:..."
		m.errText = ""
		return m, m.input.Focus()
	case actionOpenURLInput:
		m.mode = modeURLInput
		m.inputAction = inputURL
		m.inputPrompt = "Paste a .torrent URL"
		m.input.Reset()
		m.input.Placeholder = "https://example.com/file.torrent"
		m.errText = ""
		return m, m.input.Focus()
	case actionOpenImport:
		m.browser = newFileBrowser(".")
		if m.mode == modeDetail {
			m.returnMode = modeDetail
		} else {
			m.returnMode = modeMain
		}
		m.mode = modeFileBrowser
		m.errText = ""
		debug.Log("handler: entering file browser, dir=%s", m.browser.CurrentDir)
		return m, m.browser.readDirCmd()
	case actionOpenSelectFiles:
		return m.handleSelectFilesAction()
	case actionCopyURL:
		return m.handleCopyAction()
	case actionStartDownload:
		return m.handleDownloadAction()
	case actionDelete:
		return m.handleDeleteAction()
	case actionToggleBatch:
		m.batchMode = !m.batchMode
		if !m.batchMode {
			m.clearBatchSelection()
		}
		m.errText = ""
		return m, nil
	case actionExitBatch:
		m.clearBatchSelection()
		m.batchMode = false
		return m, nil
	case actionBatchMark:
		if m.batchMode && len(m.visibleTorrents()) > 0 {
			m.toggleBatchMark(m.selectedTorrentID())
		}
		return m, nil
	case actionBatchAll:
		if m.batchMode {
			m.selectAllTorrents()
		}
		return m, nil
	case actionBatchClear:
		if m.batchMode {
			m.clearBatchSelection()
		}
		return m, nil
	case actionSearchCancel:
		m.searchInput.Blur()
		m.searchInput.Reset()
		m.filterApplied = false
		m.filteredTorrents = nil
		m.selectedIdx = 0
		m.mode = modeMain
		return m, nil
	case actionSearchConfirm:
		if m.searchInput.Value() == "" {
			m.filterApplied = false
			m.filteredTorrents = nil
		} else {
			m.filterApplied = true
			m.searchInput.Blur()
		}
		m.mode = modeMain
		return m, nil
	case actionBack:
		switch m.mode {
		case modeDetail:
			m.mode = modeMain
		case modeDownload:
			m.mode = m.returnMode
			if m.download != nil && !m.download.IsTerminal() {
				m.status = "Download continues in background"
			}
		case modeFileBrowser:
			m.mode = m.returnMode
		}
		return m, nil
	case actionBrowserOpenOrToggle:
		return m, m.browser.openCurrent()
	case actionBrowserToggle:
		m.browser.toggleCurrent()
		return m, nil
	case actionBrowserEditPath:
		m.browser.startEditing()
		return m, nil
	case actionBrowserToggleVisual:
		m.browser.toggleVisual()
		return m, nil
	case actionBrowserToggleAll:
		m.browser.toggleAll()
		return m, nil
	case actionBrowserClear:
		m.browser.clearSelection()
		return m, nil
	case actionBrowserToggleHidden:
		m.browser.ShowHidden = !m.browser.ShowHidden
		return m, m.browser.readDirCmd()
	case actionBrowserUpDir:
		return m, m.browser.goUp()
	case actionBrowserPageUp:
		m.browser.pageUp(m.height / 2)
		return m, nil
	case actionBrowserPageDown:
		m.browser.pageDown(m.height / 2)
		return m, nil
	case actionBrowserTop:
		m.browser.jumpTop()
		return m, nil
	case actionBrowserBottom:
		m.browser.jumpBottom()
		return m, nil
	case actionBrowserImport:
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
	case actionBrowserEditConfirm:
		cmd, _, _, errMsg := m.browser.confirmPath()
		if errMsg != "" {
			m.errText = errMsg
		} else {
			m.errText = ""
		}
		return m, cmd
	case actionBrowserEditComplete:
		m.browser.tabComplete()
		return m, nil
	case actionBrowserEditCancel:
		m.browser.stopEditing()
		return m, m.browser.readDirCmd()
	case actionPopupCancel:
		return m.handlePopupCancel()
	case actionPopupConfirm:
		return m.handlePopupConfirm()
	case actionToggleSelection:
		if m.mode == modeSelectFiles {
			m.selector.toggleCurrent()
		}
		return m, nil
	case actionSelectAll:
		if m.mode == modeSelectFiles {
			for _, file := range m.selector.Files {
				m.selector.Selected[file.ID] = true
			}
		}
		return m, nil
	case actionClearSelection:
		if m.mode == modeSelectFiles {
			m.selector.Selected = map[int]bool{}
		}
		return m, nil
	case actionOpenFile:
		if m.canOpenManagedDownloadFile() {
			m.loading = true
			return m, openDownloadCmd(m.service, m.download.FilePath)
		}
		return m, nil
	case actionRevealFile:
		if m.canOpenManagedDownloadFile() {
			m.loading = true
			return m, revealDownloadCmd(m.service, m.download.FilePath)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleMove(delta int) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeMain, modeSearch:
		newIdx := m.selectedIdx + delta
		vis := m.visibleTorrents()
		if newIdx < 0 || newIdx >= len(vis) {
			return m, nil
		}
		m.selectedIdx = newIdx
		return m, detailCmd(m.service, m.selectedTorrentID())
	case modeFileBrowser:
		if m.browser.EditingPath {
			m.browser.moveEditCursor(delta)
		} else {
			m.browser.move(delta)
		}
		return m, nil
	case modeSelectFiles:
		newIdx := m.selector.Cursor + delta
		if newIdx < 0 || newIdx >= len(m.selector.Files) {
			return m, nil
		}
		m.selector.Cursor = newIdx
		return m, nil
	case modeChooseTarget:
		newIdx := m.targets.Cursor + delta
		if newIdx < 0 || newIdx >= len(m.targets.Items) {
			return m, nil
		}
		m.targets.Cursor = newIdx
		return m, nil
	}
	return m, nil
}

func (m Model) handleSelectFilesAction() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeMain:
		if !m.canSelectFilesFromSelection() {
			m.errText = "Selected torrent is not ready for file selection"
			return m, nil
		}
		if m.detail == nil || m.detail.ID != m.selectedTorrentID() || !isFileSelectionStatus(m.detail.Status) {
			return m.queueSelectedDetailAction(actionOpenSelectFiles)
		}
		m.returnMode = modeMain
		m.selector = newSelectFilesState(*m.detail)
		m.mode = modeSelectFiles
		return m, nil
	case modeDetail:
		if !m.canSelectFilesFromDetail() {
			m.errText = "Selected torrent is not ready for file selection"
			return m, nil
		}
		m.returnMode = modeDetail
		m.selector = newSelectFilesState(*m.detail)
		m.mode = modeSelectFiles
		return m, nil
	}
	return m, nil
}

func (m Model) handleCopyAction() (tea.Model, tea.Cmd) {
	if m.mode == modeMain && m.hasBatchSelection() {
		ids := m.batchSelectedIDs()
		m.loading = true
		m.returnMode = modeMain
		return m, batchOpCmd(m.service, batchOpCopy, ids, m.torrents)
	}
	if m.mode == modeMain {
		if !m.canReadyActionsFromSelection() {
			m.errText = "Selected torrent is not ready to download"
			return m, nil
		}
		m.returnMode = modeMain
		if m.detail == nil || m.detail.ID != m.selectedTorrentID() || m.detail.Status != "downloaded" {
			return m.queueSelectedDetailAction(actionCopyURL)
		}
		return m.beginHandoff(handoffCopy)
	}
	if m.mode == modeDetail {
		m.returnMode = modeDetail
		return m.beginHandoff(handoffCopy)
	}
	return m, nil
}

func (m Model) handleDownloadAction() (tea.Model, tea.Cmd) {
	if m.mode == modeMain {
		if m.batchMode {
			return m, nil
		}
		if m.detail != nil && m.detail.Status == "downloaded" {
			m.returnMode = modeMain
			return m.beginHandoff(handoffDownload)
		}
		if !m.canReadyActionsFromSelection() {
			m.errText = "Selected torrent is not ready to download"
			return m, nil
		}
		m.returnMode = modeMain
		if m.detail == nil || m.detail.ID != m.selectedTorrentID() || m.detail.Status != "downloaded" {
			return m.queueSelectedDetailAction(actionStartDownload)
		}
		return m.beginHandoff(handoffDownload)
	}
	if m.mode == modeDetail {
		m.returnMode = modeDetail
		return m.beginHandoff(handoffDownload)
	}
	return m, nil
}

func (m Model) handleDeleteAction() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeMain:
		if m.hasBatchSelection() {
			m.deleteIDs = m.batchSelectedIDs()
			m.returnMode = modeMain
			m.mode = modeDelete
			return m, nil
		}
		id := m.selectedTorrentID()
		if id == "" {
			return m, nil
		}
		m.returnMode = modeMain
		m.mode = modeDelete
		m.deleteIDs = []string{id}
		return m, nil
	case modeDetail:
		if m.detail == nil {
			return m, nil
		}
		m.returnMode = modeDetail
		m.mode = modeDelete
		m.deleteIDs = []string{m.detail.ID}
		return m, nil
	case modeDownload:
		if !m.canDeleteManagedDownloadTorrent() {
			return m, nil
		}
		m.returnMode = modeDownload
		m.mode = modeDelete
		m.deleteIDs = []string{m.downloadTorrentID}
		return m, nil
	}
	return m, nil
}

func (m Model) handlePopupCancel() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSelectFiles, modeChooseTarget:
		m.mode = m.returnMode
		return m, nil
	case modeDelete:
		m.mode = m.returnMode
		m.deleteIDs = nil
		return m, nil
	case modeOverwrite:
		m.mode = m.returnMode
		m.pendingDownload = nil
		m.status = "Download cancelled"
		m.errText = ""
		return m, nil
	case modeShowURL:
		m.mode = m.returnMode
		return m, nil
	case modeFileBrowser:
		m.mode = m.returnMode
		return m, nil
	}
	return m, nil
}

func (m Model) handlePopupConfirm() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSelectFiles:
		ids := m.selector.selectedIDs()
		if len(ids) == 0 {
			m.errText = "Select at least one file"
			return m, nil
		}
		m.loading = true
		return m, selectFilesCmd(m.service, m.detail.ID, ids)
	case modeChooseTarget:
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
	case modeDelete:
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
	case modeOverwrite:
		if m.pendingDownload == nil {
			m.mode = m.returnMode
			return m, nil
		}
		return m.startPendingDownload(m.pendingDownload.URL, m.pendingDownload.Filename)
	case modeShowURL:
		m.mode = m.returnMode
		return m, nil
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

func (m Model) View() tea.View {
	v := tea.NewView(renderView(m))
	v.AltScreen = true
	return v
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

func bootstrapCmd(service app.AppService) tea.Cmd {
	return func() tea.Msg {
		session, err := service.Bootstrap(context.Background())
		return bootstrapMsg{session: session, err: err}
	}
}

func authTokenCmd(service app.AppService, token string) tea.Cmd {
	return func() tea.Msg {
		session, err := service.AuthenticateWithToken(context.Background(), token, true)
		return authMsg{session: session, err: err}
	}
}

func startDeviceCmd(service app.AppService) tea.Cmd {
	return func() tea.Msg {
		code, err := service.StartDeviceFlow(context.Background())
		return deviceStartMsg{code: code, err: err}
	}
}

func pollDeviceCmd(service app.AppService, code models.DeviceCode) tea.Cmd {
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

func refreshCmd(service app.AppService) tea.Cmd {
	return func() tea.Msg {
		torrents, err := service.ListTorrents(context.Background())
		return torrentsMsg{torrents: torrents, err: err}
	}
}

func detailCmd(service app.AppService, id string) tea.Cmd {
	if id == "" {
		return nil
	}
	return func() tea.Msg {
		info, err := service.TorrentInfo(context.Background(), id)
		return detailMsg{id: id, info: info, err: err}
	}
}

func addMagnetCmd(service app.AppService, magnet string) tea.Cmd {
	return func() tea.Msg {
		result, err := service.AddMagnet(context.Background(), magnet)
		return addTorrentMsg{result: result, err: err, label: "magnet"}
	}
}

func addURLCmd(service app.AppService, remoteURL string) tea.Cmd {
	return func() tea.Msg {
		result, err := service.AddTorrentURL(context.Background(), remoteURL)
		return addTorrentMsg{result: result, err: err, label: "remote torrent URL"}
	}
}

func importCmd(service app.AppService, paths []string) tea.Cmd {
	return func() tea.Msg {
		return importMsg{results: service.ImportTorrentFiles(context.Background(), paths)}
	}
}

func selectFilesCmd(service app.AppService, torrentID string, ids []int) tea.Cmd {
	return func() tea.Msg {
		return selectFilesMsg{err: service.SelectFiles(context.Background(), torrentID, ids)}
	}
}

func deleteCmd(service app.AppService, torrentID string) tea.Cmd {
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

func batchOpCmd(service app.AppService, op batchOp, ids []string, torrents []models.Torrent) tea.Cmd {
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

func executeBatchDelete(service app.AppService, ids []string) batchResultMsg {
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

func executeBatchCopy(service app.AppService, ids []string, torrents []models.Torrent) batchResultMsg {
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

func handoffCmd(service app.AppService, target models.DownloadTarget) tea.Cmd {
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

func resolveDownloadCmd(service app.AppService, target models.DownloadTarget) tea.Cmd {
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

func startManagedDownloadCmd(service app.AppService, url string, filename string) tea.Cmd {
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

func downloadStatusCmd(service app.AppService) tea.Cmd {
	return func() tea.Msg {
		download, ok, err := service.ManagedDownloadStatus(context.Background())
		return managedDownloadStatusMsg{download: download, ok: ok, err: err}
	}
}

func openDownloadCmd(service app.AppService, path string) tea.Cmd {
	return func() tea.Msg {
		return downloadPathMsg{action: "open", err: service.OpenFile(path)}
	}
}

func revealDownloadCmd(service app.AppService, path string) tea.Cmd {
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

func modelDownloadDir(service app.AppService) string {
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

func filepathBase(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}
