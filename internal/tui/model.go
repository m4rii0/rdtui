package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/m4rii0/rdtui/internal/app"
	"github.com/m4rii0/rdtui/internal/auth"
	"github.com/m4rii0/rdtui/internal/debug"
	"github.com/m4rii0/rdtui/internal/version"
	"github.com/m4rii0/rdtui/pkg/models"
)

const refreshInterval = 5 * time.Second
const downloadRefreshInterval = time.Second

type mode string

const (
	modeStarting        mode = "starting"
	modeAuthChoice      mode = "auth-choice"
	modeTokenInput      mode = "token-input"
	modeDeviceAuth      mode = "device-auth"
	modeMain            mode = "main"
	modeDetail          mode = "detail"
	modeMagnetInput     mode = "magnet-input"
	modeURLInput        mode = "url-input"
	modeFileBrowser     mode = "file-browser"
	modeSelectFiles     mode = "select-files"
	modeChooseTarget    mode = "choose-target"
	modeOverwrite       mode = "overwrite"
	modeShowURL         mode = "show-url"
	modeDownload        mode = "download"
	modeBulkSelectFiles mode = "bulk-select-files"
	modeBulkOrder       mode = "bulk-order"
	modeBulkFiles       mode = "bulk-files"
	modeBulkConfirm     mode = "bulk-confirm"
	modeBulkDownload    mode = "bulk-download"
	modeBulkCleanup     mode = "bulk-cleanup"
	modeSearch          mode = "search"
	modeDelete          mode = "delete"
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
	batchOpDelete      batchOp = "delete"
	batchOpCopy        batchOp = "copy"
	batchOpBulkCleanup batchOp = "bulk-cleanup"
)

type batchResultMsg struct {
	op       batchOp
	total    int
	success  int
	failed   int
	skipped  int
	partial  int
	canceled int
	resolved int
	copied   int
	items    []app.ItemOutcome
	detail   string
	err      error
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
	authModel
	torrentsModel
	downloadsModel
	runtime  *app.Runtime
	setup    asyncRequest
	schedule func(time.Duration, tea.Msg) tea.Cmd

	version    string
	mode       mode
	returnMode mode
	width      int
	height     int
	loading    bool
	status     string
	errText    string

	input       textinput.Model
	inputPrompt string
	inputAction inputAction
	spinner     spinner.Model
	helpVisible bool
	flash       flashState
}

type pendingDownloadState struct {
	Queue         *app.QueueDecision
	ResumeMode    mode
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
	request requestID
	id      string
	info    models.TorrentInfo
	err     error
}

type bulkDetailsMsg struct {
	details []models.TorrentInfo
	err     error
}

type addTorrentMsg struct {
	result models.AddTorrentResult
	err    error
	label  string
}

type importMsg struct {
	result app.BatchResult
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
	ref    app.DownloadRef
	result models.ManagedDownloadStart
	err    error
}

type resolvedDownloadMsg struct {
	prepared app.PreparedDownload
	err      error
}

type managedDownloadStatusMsg struct {
	request  downloadRequestID
	queue    queueIdentity
	download models.ManagedDownload
	ok       bool
	err      error
}

type downloadPathMsg struct {
	action string
	err    error
}

type tickMsg struct{ request requestID }

type devicePollTickMsg struct{ request requestID }

type downloadTickMsg struct{ request downloadRequestID }

func NewModel(deps Dependencies) Model {
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
		authService:     deps.Auth,
		authState:       deps.AuthState,
		torrentService:  deps.Torrents,
		downloads:       deps.Downloads,
		batches:         deps.Batches,
		desktop:         deps.Desktop,
		prepareDownload: deps.PrepareDownload,
		bulkQueue:       deps.Bulk,
		runtime:         deps.Runtime,
		version:         version.Current(),
		mode:            modeStarting,
		returnMode:      modeMain,
		input:           ti,
		searchInput:     si,
		spinner:         sp,
		progress:        pb,
		sortColumn:      colAdded,
		sortAscending:   false,
		batchSelected:   map[string]bool{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(bootstrapCmd(m.runtime.Context(), m.runtime, m.authService), m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(featureResultMsg); ok {
		state := m.feature(result.feature)
		if !state.current(result.request) || (result.feature == setupFeature && result.queue != m.queueIdentity()) {
			return m, nil
		}
		state.InFlight = false
		if result.feature != authFeature {
			state.invalidate()
		}
		msg = result.msg
	}
	if err := m.authenticationRequired(msg); err != nil {
		if snapshot, ok := msg.(bulkSnapshotMsg); ok && snapshot.err == nil {
			// Accept terminal outcomes before leaving for authentication. The
			// retained snapshot acknowledges them without erasing failure history.
			updated, _ := m.handleBulkSnapshot(snapshot)
			m = updated.(Model)
		}
		m.auth.invalidate()
		m.list.invalidate()
		m.detailRequest.invalidate()
		m.setup.invalidate()
		m.downloadPoll.invalidate()
		m.queuePoll.invalidate()
		m.session, m.deviceCode = nil, nil
		m.loading, m.mode = false, modeAuthChoice
		m.status, m.errText = "Choose an authentication method", err.Error()
		return m, nil
	}
	switch msg := msg.(type) {
	case bootstrapMsg, authMsg, deviceStartMsg, devicePollMsg, devicePollTickMsg:
		cmd := m.authModel.update(&m, msg)
		return m, cmd
	case tickMsg, torrentsMsg, detailMsg, browserReadDirMsg, addTorrentMsg,
		importMsg, selectFilesMsg, bulkFileSelectionDetailsMsg, bulkFileSelectionResultMsg,
		deleteMsg, batchResultMsg:
		return m.torrentsModel.update(&m, msg)
	case bulkStartedMsg, bulkSnapshotMsg, bulkTickMsg, bulkDecisionMsg,
		downloadTickMsg, managedDownloadMsg, resolvedDownloadMsg,
		managedDownloadStatusMsg, downloadPathMsg, progress.FrameMsg, bulkDetailsMsg, handoffMsg:
		return m.downloadsModel.update(&m, msg)
	case operationErrorMsg:
		m.loading = false
		m.errText = msg.err.Error()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		debug.Log("Update: WindowSize %dx%d", msg.Width, msg.Height)
		return m, nil

	case flashTimeoutMsg:
		m.clearFlash()
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

	}

	var cmd tea.Cmd
	if m.needsTextInputUpdate() {
		m.input, cmd = m.input.Update(msg)
	} else if m.mode == modeSearch {
		m.searchInput, cmd = m.searchInput.Update(msg)
	}
	return m, cmd
}

// Authentication is a root navigation concern. Preserve each feature's stale
// result checks before interpreting a rejection, including per-item batch errors.
func (m Model) authenticationRequired(msg tea.Msg) error {
	var err error
	var items []app.ItemOutcome
	switch msg := msg.(type) {
	case torrentsMsg:
		err = msg.err
	case detailMsg:
		if !m.detailRequest.current(msg.request) || msg.id != m.selectedTorrentID() {
			return nil
		}
		err = msg.err
	case addTorrentMsg:
		err = msg.err
	case selectFilesMsg:
		err = msg.err
	case deleteMsg:
		err = msg.err
	case importMsg:
		err, items = msg.result.Err, msg.result.Items
	case batchResultMsg:
		err, items = msg.err, msg.items
	case bulkFileSelectionResultMsg:
		err, items = msg.err, msg.items
	case bulkFileSelectionDetailsMsg:
		err = msg.err
	case bulkDetailsMsg:
		err = msg.err
	case resolvedDownloadMsg:
		err = msg.err
	case handoffMsg:
		err = msg.err
	case operationErrorMsg:
		err = msg.err
	case bulkStartedMsg:
		if m.bulk == nil || msg.generation != m.queueGeneration {
			return nil
		}
		err = msg.err
	case bulkSnapshotMsg:
		if !m.acceptsBulkSnapshot(msg) {
			return nil
		}
		err = msg.err
		previous := m.bulk.Snapshot
		for index, item := range msg.snapshot.Items {
			if item.Outcome != nil {
				// Outcomes are immutable per queue/item. Previously accepted
				// failures are history, not new rejections of a replacement session.
				if previous.ID == msg.queueID && index < len(previous.Items) && previous.Items[index].Outcome != nil {
					continue
				}
				items = append(items, *item.Outcome)
			}
		}
	}
	for _, item := range items {
		err = errors.Join(err, item.Err)
	}
	if errors.Is(err, auth.ErrNoValidCredentials) {
		// Application-owned queues may finish unseen items while login is in
		// progress. Their immutable errors cannot reject a session installed
		// since then; ask the credential owner, never the old UI snapshot.
		if m.authState != nil && m.authState.Snapshot().Method != models.AuthMethodNone {
			return nil
		}
		return err
	}
	return nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	debug.Log("handleKey: key=%q mode=%s", msg.String(), m.mode)
	if msg.String() == "ctrl+c" {
		m.runtime.Stop()
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
			m.runtime.Stop()
			return m, tea.Quit
		case "1", "t":
			m.auth.invalidate()
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
			cmd := m.beginFeature(authFeature, func(ctx context.Context) tea.Cmd { return startDeviceCmd(ctx, m.runtime, m.authService) })
			return m, cmd
		}

	case modeTokenInput, modeMagnetInput, modeURLInput:
		switch msg.String() {
		case "esc":
			if m.inputAction == inputToken {
				m.auth.invalidate()
				m.loading = false
			}
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
				cmd := m.beginFeature(authFeature, func(ctx context.Context) tea.Cmd { return authTokenCmd(ctx, m.runtime, m.authService, value) })
				return m, cmd
			case inputMagnet:
				return m, addMagnetCmd(m.runtime.Context(), m.runtime, m.torrentService, value)
			case inputURL:
				return m, addURLCmd(m.runtime.Context(), m.runtime, m.torrentService, value)
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case modeDeviceAuth:
		switch msg.String() {
		case "esc":
			m.auth.invalidate()
			m.loading = false
			m.mode = modeAuthChoice
			m.deviceCode = nil
			return m, nil
		case "enter", "r":
			if m.deviceCode != nil {
				cmd := m.pollDevice()
				return m, cmd
			}
		}

	case modeBulkDownload:
		if msg.Code == tea.KeyEscape || msg.String() == "esc" {
			return m.handleShortcutAction(actionBack)
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
		m.runtime.Stop()
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
			cmd := m.requestDetail(m.selectedTorrentID())
			return m, cmd
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
		case modeDownload, modeBulkDownload:
			if m.mode == modeBulkDownload && m.bulk != nil && !m.bulk.isFinished() {
				cmd := m.pollBulkQueue()
				return m, cmd
			}
			if m.mode == modeBulkDownload && (m.bulk == nil || m.bulk.isFinished()) {
				m.loading = true
				cmd := m.refresh()
				return m, cmd
			}
			cmd := m.pollDownload()
			if cmd != nil {
				m.loading = true
			}
			return m, cmd
		default:
			m.loading = true
			cmd := m.refresh()
			return m, cmd
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
	case actionMoveItemUp:
		if m.mode == modeBulkOrder && m.bulk != nil {
			m.bulk.moveOrderItem(-1)
		}
		return m, nil
	case actionMoveItemDown:
		if m.mode == modeBulkOrder && m.bulk != nil {
			m.bulk.moveOrderItem(1)
		}
		return m, nil
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
			m.detailRequest.invalidate()
			m.mode = modeMain
		case modeDownload:
			m.mode = m.returnMode
			if m.download != nil && !m.download.IsTerminal() {
				m.status = "Download continues in background"
			}
		case modeBulkDownload:
			m.mode = modeMain
			if m.bulk != nil && !m.bulk.isFinished() {
				m.status = "Bulk download continues in background"
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
		return m, importCmd(m.runtime.Context(), m.runtime, m.batches, valid)
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
		} else if m.mode == modeBulkSelectFiles && m.bulkSelect != nil {
			m.bulkSelect.toggleCurrentFile()
		} else if m.mode == modeBulkFiles && m.bulk != nil {
			m.bulk.toggleCurrentFile()
		} else if m.mode == modeBulkCleanup && m.bulk != nil {
			m.bulk.toggleCleanupSelection()
		}
		return m, nil
	case actionSelectAll:
		if m.mode == modeSelectFiles {
			for _, file := range m.selector.Files {
				m.selector.Selected[file.ID] = true
			}
		} else if m.mode == modeBulkSelectFiles && m.bulkSelect != nil {
			m.bulkSelect.selectAllCurrentFiles()
		} else if m.mode == modeBulkFiles && m.bulk != nil {
			m.bulk.selectAllCurrentFiles()
		} else if m.mode == modeBulkCleanup && m.bulk != nil {
			m.bulk.selectAllCleanup()
		}
		return m, nil
	case actionClearSelection:
		if m.mode == modeSelectFiles {
			m.selector.Selected = map[int]bool{}
		} else if m.mode == modeBulkSelectFiles && m.bulkSelect != nil {
			m.bulkSelect.clearCurrentFiles()
		} else if m.mode == modeBulkFiles && m.bulk != nil {
			m.bulk.clearCurrentFiles()
		} else if m.mode == modeBulkCleanup && m.bulk != nil {
			m.bulk.clearCleanup()
		}
		return m, nil
	case actionOpenFile:
		if m.canOpenManagedDownloadFile() {
			m.loading = true
			return m, openDownloadCmd(m.runtime.Context(), m.runtime, m.desktop, m.download.FilePath)
		}
		return m, nil
	case actionRevealFile:
		if m.canOpenManagedDownloadFile() {
			m.loading = true
			return m, revealDownloadCmd(m.runtime.Context(), m.runtime, m.desktop, m.download.FilePath)
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
		cmd := m.requestDetail(m.selectedTorrentID())
		return m, cmd
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
	case modeBulkSelectFiles:
		if m.bulkSelect != nil {
			m.bulkSelect.moveFileCursor(delta)
		}
		return m, nil
	case modeBulkOrder:
		if m.bulk != nil {
			m.bulk.moveOrderCursor(delta)
		}
		return m, nil
	case modeBulkFiles:
		if m.bulk != nil {
			m.bulk.moveFileCursor(delta)
		}
		return m, nil
	case modeBulkCleanup:
		if m.bulk != nil {
			m.bulk.moveCleanupCursor(delta)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleSelectFilesAction() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeMain:
		if m.batchMode {
			if !m.canBulkSelectFilesSelection() {
				m.errText = "Mark at least one torrent that needs file selection"
				return m, nil
			}
			return m.beginBulkFileSelectionSetup()
		}
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
		return m, batchOpCmd(m.runtime.Context(), m.runtime, m.batches, batchOpCopy, ids, m.torrents)
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
	if m.bulk != nil && m.bulk.QueueID != 0 && !m.bulk.isFinished() {
		m.mode = modeBulkDownload
		return m, nil
	}
	if m.mode == modeMain {
		if m.batchMode {
			if !m.canBulkDownloadSelection() {
				m.errText = "Mark at least two downloaded torrents to bulk download"
				return m, nil
			}
			return m.beginBulkDownloadSetup()
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
	case modeBulkDownload:
		if m.bulk == nil || !m.bulk.isFinished() {
			return m, nil
		}
		m.returnMode = modeBulkDownload
		m.bulk.initCleanupSelection()
		m.mode = modeBulkCleanup
		return m, nil
	}
	return m, nil
}

func (m Model) handlePopupCancel() (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSelectFiles, modeChooseTarget:
		m.setup.invalidate()
		m.loading = false
		m.mode = m.returnMode
		return m, nil
	case modeBulkSelectFiles:
		m.setup.invalidate()
		m.loading = false
		m.mode = modeMain
		m.bulkSelect = nil
		m.errText = ""
		return m, nil
	case modeBulkOrder, modeBulkFiles, modeBulkConfirm:
		m.setup.invalidate()
		m.loading = false
		m.mode = modeMain
		m.bulk = nil
		m.errText = ""
		return m, nil
	case modeBulkCleanup:
		m.mode = modeBulkDownload
		m.errText = ""
		return m, nil
	case modeDelete:
		m.mode = m.returnMode
		m.deleteIDs = nil
		return m, nil
	case modeOverwrite:
		pending := m.pendingDownload
		m.pendingDownload = nil
		if pending != nil && pending.Queue != nil {
			m.mode = pending.ResumeMode
			return m.decideBulkOverwrite(*pending.Queue, false)
		}
		m.setup.invalidate()
		m.mode = m.returnMode
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
		return m, selectFilesCmd(m.runtime.Context(), m.runtime, m.torrentService, m.detail.ID, ids)
	case modeChooseTarget:
		if len(m.targets.Items) == 0 {
			return m, nil
		}
		if m.targets.Action == handoffDownload && m.detail != nil {
			m.downloadTorrentID = m.detail.ID
		}
		m.loading = true
		if m.targets.Action == handoffDownload {
			cmd := m.resolveDownload(m.targets.Items[m.targets.Cursor])
			return m, cmd
		}
		cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd {
			return handoffCmd(ctx, m.runtime, m.batches, m.targets.Items[m.targets.Cursor])
		})
		return m, cmd
	case modeBulkOrder:
		if m.bulk == nil || len(m.bulk.Plans) == 0 {
			return m, nil
		}
		m.loading = true
		m.errText = ""
		cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd {
			return bulkDetailsCmd(ctx, m.runtime, m.torrentService, m.bulk.Plans)
		})
		return m, cmd
	case modeBulkFiles:
		if m.bulk == nil {
			return m, nil
		}
		if m.bulk.selectedCurrentFileCount() == 0 {
			m.errText = "Select at least one file"
			return m, nil
		}
		nextStart := m.bulk.FilePrompt + 1
		if m.bulk.prepareNextFilePrompt(nextStart) {
			m.errText = ""
			return m, nil
		}
		m.bulk.buildQueueItems()
		m.mode = modeBulkConfirm
		m.errText = ""
		return m, nil
	case modeBulkSelectFiles:
		if m.bulkSelect == nil {
			return m, nil
		}
		if m.bulkSelect.selectedCurrentFileCount() == 0 {
			m.errText = "Select at least one file"
			return m, nil
		}
		nextStart := m.bulkSelect.Prompt + 1
		if m.bulkSelect.prepareNextPrompt(nextStart) {
			m.errText = ""
			return m, nil
		}
		return m.startBulkFileSelectionSubmit()
	case modeBulkConfirm:
		if m.bulk == nil {
			return m, nil
		}
		if len(m.bulk.Items) == 0 {
			m.errText = "No files selected for bulk download"
			return m, nil
		}
		m.returnMode = modeBulkDownload
		m.mode = modeBulkDownload
		m.clearBatchSelection()
		m.batchMode = false
		m.download = nil
		m.status = "Starting bulk download"
		m.errText = ""
		return m.startBulkQueue()
	case modeBulkCleanup:
		if m.bulk == nil {
			return m, nil
		}
		ids := m.bulk.cleanupSelectedIDs()
		if len(ids) == 0 {
			m.errText = "Select at least one source torrent"
			return m, nil
		}
		m.loading = true
		return m, batchOpCmd(m.runtime.Context(), m.runtime, m.batches, batchOpBulkCleanup, ids, m.torrents)
	case modeDelete:
		if len(m.deleteIDs) > 1 {
			m.loading = true
			ids := make([]string, len(m.deleteIDs))
			copy(ids, m.deleteIDs)
			return m, batchOpCmd(m.runtime.Context(), m.runtime, m.batches, batchOpDelete, ids, m.torrents)
		}
		if len(m.deleteIDs) == 0 {
			m.mode = m.returnMode
			return m, nil
		}
		m.loading = true
		return m, deleteCmd(m.runtime.Context(), m.runtime, m.torrentService, m.deleteIDs[0])
	case modeOverwrite:
		if m.pendingDownload == nil {
			m.mode = m.returnMode
			return m, nil
		}
		if m.pendingDownload.Queue != nil {
			pending := m.pendingDownload
			m.pendingDownload = nil
			m.mode = pending.ResumeMode
			return m.decideBulkOverwrite(*pending.Queue, true)
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
	m.setup.invalidate()
	if len(targets) == 1 {
		if action == handoffDownload {
			if m.detail != nil {
				m.downloadTorrentID = m.detail.ID
			}
			m.loading = true
			cmd := m.resolveDownload(targets[0])
			return *m, cmd
		}
		m.loading = true
		cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd { return handoffCmd(ctx, m.runtime, m.batches, targets[0]) })
		return *m, cmd
	}
	m.targets = targetPickerState{Items: targets, Action: action}
	m.mode = modeChooseTarget
	return *m, nil
}

func (m Model) beginBulkDownloadSetup() (tea.Model, tea.Cmd) {
	if m.bulk != nil && (m.bulk.QueueID != 0 && !m.bulk.isFinished() || m.bulk.Starting) {
		m.mode = modeBulkDownload
		return m, nil
	}
	selected := make([]models.Torrent, 0, len(m.batchSelected))
	for _, torrent := range m.visibleTorrents() {
		if m.batchSelected[torrent.ID] {
			selected = append(selected, torrent)
		}
	}
	if len(selected) < 2 {
		m.errText = "Mark at least two downloaded torrents to bulk download"
		return m, nil
	}
	m.bulk = newBulkDownloadState(selected)
	m.setup.invalidate()
	m.queueGeneration++
	m.returnMode = modeMain
	m.mode = modeBulkOrder
	m.errText = ""
	m.status = "Review bulk download order"
	return m, nil
}

func (m Model) beginBulkFileSelectionSetup() (tea.Model, tea.Cmd) {
	eligible, skipped := m.bulkFileSelectionCandidates()
	if len(eligible) == 0 {
		m.errText = "Mark at least one torrent that needs file selection"
		return m, nil
	}
	m.bulkSelect = newBulkFileSelectionState(eligible, skipped)
	m.returnMode = modeMain
	m.mode = modeBulkSelectFiles
	m.loading = true
	m.errText = ""
	m.status = "Loading bulk file selection details"
	cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd {
		return bulkFileSelectionDetailsCmd(ctx, m.runtime, m.torrentService, m.bulkSelect.Plans)
	})
	return m, cmd
}

func (m Model) bulkFileSelectionCandidates() (eligible []models.Torrent, skipped []models.Torrent) {
	for _, torrent := range m.visibleTorrents() {
		if !m.batchSelected[torrent.ID] {
			continue
		}
		if isFileSelectionStatus(torrent.Status) {
			eligible = append(eligible, torrent)
		} else {
			skipped = append(skipped, torrent)
		}
	}
	return eligible, skipped
}

func (m Model) startBulkFileSelectionSubmit() (tea.Model, tea.Cmd) {
	if m.bulkSelect == nil {
		return m, nil
	}
	submissions := m.bulkSelect.submissions()
	m.mode = modeMain
	m.loading = true
	m.status = "Submitting bulk file selections"
	m.errText = ""
	return m, bulkFileSelectionSubmitCmd(m.runtime.Context(), m.runtime, m.batches, submissions, m.bulkSelect.Outcomes)
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
	clear(m.batchSelected)
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

type operationErrorMsg struct{ err error }

// Admission happens when Tea executes the command, not when it is queued.
func operationCmd(ctx context.Context, runtime *app.Runtime, fn func(context.Context) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		var msg tea.Msg
		if err := runtime.Do(ctx, func(ctx context.Context) error {
			msg = fn(ctx)
			return nil
		}); err != nil {
			return operationErrorMsg{err: err}
		}
		return msg
	}
}

func selectedIndexForID(torrents []models.Torrent, id string) int {
	if len(torrents) == 0 {
		return 0
	}
	if idx, ok := torrentIndexForID(torrents, id); ok {
		return idx
	}
	return 0
}

func torrentIndexForID(torrents []models.Torrent, id string) (int, bool) {
	if id == "" {
		return 0, false
	}
	for idx, torrent := range torrents {
		if torrent.ID == id {
			return idx, true
		}
	}
	return 0, false
}

func fallbackModeForInput(action inputAction) mode {
	if action == inputToken {
		return modeAuthChoice
	}
	return modeMain
}

func filepathBase(path string) string {
	normalized := strings.ReplaceAll(path, "\\", "/")
	if _, base, found := strings.CutLast(normalized, "/"); found {
		return base
	}
	return normalized
}
