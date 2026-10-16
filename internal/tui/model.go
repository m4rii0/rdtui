package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	service         app.AppService
	runtime         *app.Runtime
	auth            asyncRequest
	list            asyncRequest
	setup           asyncRequest
	detailRequest   asyncRequest
	downloadPoll    asyncRequest
	downloadRef     app.DownloadRef
	queueGeneration uint64
	schedule        func(time.Duration, tea.Msg) tea.Cmd

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
	pendingFocusID   string
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
	bulk              *bulkDownloadState
	bulkSelect        *bulkFileSelectionState
	deleteIDs         []string
	helpVisible       bool
	pendingAction     *pendingShortcutAction
	flash             flashState
}

type pendingDownloadState struct {
	Queue         *queueIdentity
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
	url      string
	filename string
	filesize int64
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

func NewModel(service app.AppService, runtime *app.Runtime) Model {
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
		runtime:       runtime,
		version:       version.Current(),
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
	return tea.Batch(bootstrapCmd(m.runtime.Context(), m.runtime, m.service), m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	queueResult := false
	if result, ok := msg.(featureResultMsg); ok {
		state := m.feature(result.feature)
		if !state.current(result.request) || (result.feature == setupFeature && result.queue != m.queueIdentity()) {
			return m, nil
		}
		// Route by the admitted request's owner, not mutable navigation state.
		queueResult = result.feature == setupFeature && m.bulk != nil && m.bulk.isRunning() && result.queue == m.queueIdentity()
		state.InFlight = false
		if result.feature != authFeature {
			state.invalidate()
		}
		msg = result.msg
	}
	switch msg := msg.(type) {
	case operationErrorMsg:
		m.loading = false
		m.errText = msg.err.Error()
		return m, nil
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
			cmd := m.refresh()
			return m, cmd
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
		m.auth.invalidate()
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
		cmd := m.refresh()
		return m, cmd

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
		return m, m.after(time.Duration(max(1, msg.code.Interval))*time.Second, devicePollTickMsg{requestID{m.auth.Generation}})

	case devicePollMsg:
		if msg.err == nil {
			m.auth.invalidate()
			m.session = msg.session
			m.mode = modeMain
			m.deviceCode = nil
			m.status = "Authenticated as " + msg.session.User.Username
			m.errText = ""
			cmd := m.refresh()
			return m, cmd
		}
		if errors.Is(msg.err, realdebrid.ErrAuthorizationPending) {
			if m.deviceCode != nil && time.Since(m.deviceCode.RequestedAt) < time.Duration(m.deviceCode.ExpiresIn)*time.Second {
				return m, m.after(time.Duration(max(1, m.deviceCode.Interval))*time.Second, devicePollTickMsg{requestID{m.auth.Generation}})
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
		if !m.list.current(msg.request) || m.list.InFlight {
			return m, nil
		}
		if m.mode == modeMain || m.mode == modeDetail || m.mode == modeSearch {
			cmd := m.refresh()
			return m, cmd
		}
		return m, m.nextListPoll()

	case devicePollTickMsg:
		if !m.auth.current(msg.request) || m.auth.InFlight {
			return m, nil
		}
		if m.deviceCode != nil && m.mode == modeDeviceAuth {
			cmd := m.pollDevice()
			return m, cmd
		}
		return m, nil

	case downloadTickMsg:
		if !m.downloadPoll.current(msg.request.Request) || msg.request.Ref != m.downloadRef {
			return m, nil
		}
		if m.bulk != nil && m.bulk.isRunning() && m.download != nil && !m.download.IsTerminal() {
			cmd := m.pollDownload()
			return m, cmd
		}
		if m.mode != modeDownload || m.download == nil || m.download.IsTerminal() {
			return m, nil
		}
		cmd := m.pollDownload()
		return m, cmd

	case flashTimeoutMsg:
		m.clearFlash()
		return m, nil

	case torrentsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, m.nextListPoll()
		}
		selectedID := m.selectedTorrentID()
		m.torrents = slices.Clone(msg.torrents)
		sortTorrents(m.torrents, m.sortColumn, m.sortAscending)
		if m.filterApplied || m.mode == modeSearch {
			m.applyFilter()
		}
		vis := m.visibleTorrents()
		if m.pendingFocusID != "" {
			if idx, ok := torrentIndexForID(vis, m.pendingFocusID); ok {
				m.selectedIdx = idx
				m.pendingFocusID = ""
			} else {
				m.selectedIdx = selectedIndexForID(vis, selectedID)
			}
		} else {
			m.selectedIdx = selectedIndexForID(vis, selectedID)
		}
		m.errText = ""
		if len(m.torrents) == 0 {
			m.detailRequest.invalidate()
			m.detail = nil
			m.status = "No torrents found"
			return m, m.nextListPoll()
		}
		cmd := m.requestDetail(m.selectedTorrentID())
		return m, tea.Batch(cmd, m.nextListPoll())

	case detailMsg:
		if !m.detailRequest.current(msg.request) || msg.id != m.selectedTorrentID() {
			return m, nil
		}
		m.detailRequest.invalidate()
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

	case bulkDetailsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		if m.bulk == nil {
			return m, nil
		}
		m.bulk.applyDetails(msg.details)
		if m.bulk.prepareFirstFilePrompt() {
			m.mode = modeBulkFiles
			m.errText = ""
			return m, nil
		}
		m.bulk.buildQueueItems()
		m.mode = modeBulkConfirm
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
		m.pendingFocusID = msg.result.ID
		m.errText = ""
		cmd := m.refresh()
		return m, cmd

	case importMsg:
		m.loading = false
		m.mode = modeMain
		success := 0
		failed := 0
		canceled := 0
		failedNames := []string{}
		focusID := ""
		for _, result := range msg.result.Items {
			if result.Kind == app.OutcomeSuccess {
				success++
				if focusID == "" {
					focusID = result.TorrentCreatedID
				}
			} else if result.Kind == app.OutcomeCanceled {
				canceled++
			} else {
				failed++
				failedNames = append(failedNames, filepathBase(result.Source))
			}
		}
		if focusID != "" {
			m.pendingFocusID = focusID
		}
		if failed == 0 {
			m.status = fmt.Sprintf("Imported %d torrent file(s)", success)
			m.errText = ""
		} else {
			m.status = fmt.Sprintf("Imported %d torrent file(s), %d failed", success, failed)
			m.errText = "Failed: " + strings.Join(failedNames, ", ")
		}
		if canceled > 0 {
			m.status += fmt.Sprintf(", %d canceled", canceled)
			m.errText = strings.TrimSpace(m.errText + "\n" + fmt.Sprint(msg.result.Err))
			for _, item := range msg.result.Items {
				if item.RemoteOutcomeUnknown {
					m.errText += "\nRemote outcome unknown; not replayed"
					break
				}
			}
		}
		cmd := m.refresh()
		return m, cmd

	case selectFilesMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.mode = m.returnMode
		m.errText = ""
		cmd := m.refresh()
		return m, tea.Batch(cmd, m.setFlash(flashSuccess, "Updated torrent file selection"))

	case bulkFileSelectionDetailsMsg:
		m.loading = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		if m.bulkSelect == nil {
			return m, nil
		}
		m.bulkSelect.applyDetails(msg.details, msg.outcomes)
		if m.bulkSelect.prepareFirstPrompt() {
			m.mode = modeBulkSelectFiles
			m.errText = ""
			return m, nil
		}
		return m.startBulkFileSelectionSubmit()

	case bulkFileSelectionResultMsg:
		m.loading = false
		m.mode = modeMain
		m.bulkSelect = nil
		m.clearBatchSelection()
		m.batchMode = false
		flashMsg := "Bulk file selection complete: " + bulkFileSelectionSummaryLine(msg.success, msg.failed, msg.skipped)
		if msg.canceled > 0 {
			flashMsg += fmt.Sprintf(", %d canceled", msg.canceled)
		}
		if msg.detail != "" {
			m.errText = strings.TrimSpace(msg.detail)
		} else {
			m.errText = ""
		}
		level := flashSuccess
		if msg.failed > 0 || msg.canceled > 0 {
			level = flashWarn
		}
		cmd := m.refresh()
		return m, tea.Batch(cmd, m.setFlash(level, flashMsg))

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
		cmd := m.refresh()
		return m, tea.Batch(cmd, m.setFlash(flashSuccess, "Deleted torrent"))

	case batchResultMsg:
		m.loading = false
		switch msg.op {
		case batchOpDelete:
			m.mode = modeMain
			var flashMsg string
			if msg.failed > 0 || msg.canceled > 0 {
				flashMsg = fmt.Sprintf("Deleted %d/%d torrent(s)", msg.success, msg.total)
			} else {
				flashMsg = fmt.Sprintf("Deleted %d torrent(s)", msg.success)
			}
			if msg.canceled > 0 {
				flashMsg += fmt.Sprintf(", %d canceled", msg.canceled)
			}
			if msg.detail != "" {
				m.errText = strings.TrimSpace(msg.detail)
			} else {
				m.errText = ""
			}
			m.deleteIDs = nil
			m.clearBatchSelection()
			m.batchMode = false
			cmd := m.refresh()
			return m, tea.Batch(cmd, m.setFlash(flashSuccess, flashMsg))
		case batchOpCopy:
			m.mode = modeMain
			var parts []string
			if msg.copied == msg.resolved && msg.copied > 0 {
				parts = append(parts, fmt.Sprintf("Copied %d URL(s)", msg.copied))
			} else {
				parts = append(parts, fmt.Sprintf("Resolved %d URL(s), copied %d", msg.resolved, msg.copied))
			}
			if msg.success > 0 {
				parts = append(parts, fmt.Sprintf("%d successful torrent(s)", msg.success))
			}
			if msg.partial > 0 {
				parts = append(parts, fmt.Sprintf("%d partial torrent(s)", msg.partial))
			}
			if msg.skipped > 0 {
				parts = append(parts, fmt.Sprintf("%d skipped torrent(s)", msg.skipped))
			}
			if msg.failed > 0 {
				parts = append(parts, fmt.Sprintf("%d failed torrent(s)", msg.failed))
			}
			if msg.canceled > 0 {
				parts = append(parts, fmt.Sprintf("%d canceled torrent(s)", msg.canceled))
			}
			flashMsg := strings.Join(parts, ", ")
			if msg.detail != "" {
				m.errText = strings.TrimSpace(msg.detail)
			} else {
				m.errText = ""
			}
			m.clearBatchSelection()
			m.batchMode = false
			level := flashSuccess
			if msg.partial > 0 || msg.failed > 0 || msg.canceled > 0 || msg.copied < msg.resolved {
				level = flashWarn
			}
			return m, m.setFlash(level, flashMsg)
		case batchOpBulkCleanup:
			m.mode = modeBulkDownload
			var flashMsg string
			if msg.failed > 0 || msg.canceled > 0 {
				flashMsg = fmt.Sprintf("Deleted %d/%d source torrent(s)", msg.success, msg.total)
			} else {
				flashMsg = fmt.Sprintf("Deleted %d source torrent(s)", msg.success)
			}
			if msg.canceled > 0 {
				flashMsg += fmt.Sprintf(", %d canceled", msg.canceled)
			}
			if msg.detail != "" {
				m.errText = strings.TrimSpace(msg.detail)
			} else {
				m.errText = ""
			}
			cmd := m.refresh()
			return m, tea.Batch(cmd, m.setFlash(flashSuccess, flashMsg))
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
		m.downloadPoll.invalidate()
		m.downloadRef = msg.ref
		m.loading = false
		if queueResult {
			foreground := m.mode == modeBulkDownload || m.mode == modeBulkConfirm
			if msg.err != nil {
				m.bulk.finishCurrent(bulkFileFailed, "", msg.err.Error())
				return m.startNextBulkItem()
			}
			download := msg.result.Download
			m.download = &download
			if foreground {
				m.mode = modeBulkDownload
			}
			m.errText = ""
			if msg.result.Reused {
				m.status = "Reopened active bulk download: " + download.Filename
			} else {
				m.status = "Started bulk download: " + download.Filename
			}
			if download.IsTerminal() {
				if download.IsComplete() {
					m.bulk.finishCurrent(bulkFileSuccess, download.Filename, "")
				} else {
					m.bulk.finishCurrent(bulkFileFailed, download.Filename, download.ErrorMessage)
				}
				return m.startNextBulkItem()
			}
			cmd := m.pollDownload()
			return m, cmd
		}
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
		cmd := m.pollDownload()
		return m, cmd

	case resolvedDownloadMsg:
		m.loading = false
		if msg.err != nil {
			if queueResult {
				m.bulk.finishCurrent(bulkFileFailed, "", msg.err.Error())
				return m.startNextBulkItem()
			}
			m.errText = msg.err.Error()
			return m, nil
		}
		path := filepath.Join(m.downloadDir, msg.filename)
		existingBytes, exists, err := existingFileSize(path)
		if err != nil {
			if queueResult {
				m.bulk.finishCurrent(bulkFileFailed, msg.filename, err.Error())
				return m.startNextBulkItem()
			}
			m.errText = err.Error()
			return m, nil
		}
		if exists {
			m.pendingDownload = &pendingDownloadState{
				ResumeMode:    m.mode,
				URL:           msg.url,
				Filename:      msg.filename,
				Path:          path,
				RemoteBytes:   msg.filesize,
				ExistingBytes: existingBytes,
			}
			if queueResult {
				m.pendingDownload.Queue = new(m.queueIdentity())
			}
			m.mode = modeOverwrite
			m.errText = ""
			m.status = "Existing file found"
			return m, nil
		}
		return m.startPendingDownload(msg.url, msg.filename)

	case managedDownloadStatusMsg:
		if !m.downloadPoll.current(msg.request.Request) || msg.request.Ref != m.downloadRef || msg.queue != m.queueIdentity() {
			return m, nil
		}
		m.downloadPoll.invalidate()
		m.loading = false
		if !msg.ok || (msg.err != nil && !msg.download.IsTerminal()) {
			// Observation failures never replace the last valid snapshot.
			if m.download != nil && !m.download.IsTerminal() {
				return m, m.nextDownloadPoll()
			}
			return m, nil
		}
		download := msg.download
		m.download = &download
		if m.bulk != nil && m.bulk.isRunning() {
			if msg.err != nil {
				errText := msg.err.Error()
				if download.ErrorMessage != "" {
					errText = download.ErrorMessage
				}
				m.bulk.finishCurrent(bulkFileFailed, download.Filename, errText)
				return m.startNextBulkItem()
			}
			if download.IsComplete() {
				m.bulk.finishCurrent(bulkFileSuccess, download.Filename, "")
				return m.startNextBulkItem()
			}
			if download.IsError() {
				m.bulk.finishCurrent(bulkFileFailed, download.Filename, download.ErrorMessage)
				return m.startNextBulkItem()
			}
			return m, m.nextDownloadPoll()
		}
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
		return m, m.nextDownloadPoll()

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
			cmd := m.beginFeature(authFeature, func(ctx context.Context) tea.Cmd { return startDeviceCmd(ctx, m.runtime, m.service) })
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
				cmd := m.beginFeature(authFeature, func(ctx context.Context) tea.Cmd { return authTokenCmd(ctx, m.runtime, m.service, value) })
				return m, cmd
			case inputMagnet:
				return m, addMagnetCmd(m.runtime.Context(), m.runtime, m.service, value)
			case inputURL:
				return m, addURLCmd(m.runtime.Context(), m.runtime, m.service, value)
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
		return m, importCmd(m.runtime.Context(), m.runtime, m.service, valid)
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
			return m, openDownloadCmd(m.runtime.Context(), m.runtime, m.service, m.download.FilePath)
		}
		return m, nil
	case actionRevealFile:
		if m.canOpenManagedDownloadFile() {
			m.loading = true
			return m, revealDownloadCmd(m.runtime.Context(), m.runtime, m.service, m.download.FilePath)
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
		return m, batchOpCmd(m.runtime.Context(), m.runtime, m.service, batchOpCopy, ids, m.torrents)
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
			if m.bulk == nil || !m.bulk.isRunning() || *pending.Queue != m.queueIdentity() {
				return m, nil
			}
			m.setup.invalidate()
			m.bulk.finishCurrent(bulkFileSkipped, pending.Filename, "existing file skipped")
			m.errText = ""
			return m.startNextBulkItem()
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
		return m, selectFilesCmd(m.runtime.Context(), m.runtime, m.service, m.detail.ID, ids)
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
			return handoffCmd(ctx, m.runtime, m.service, m.targets.Items[m.targets.Cursor])
		})
		return m, cmd
	case modeBulkOrder:
		if m.bulk == nil || len(m.bulk.Plans) == 0 {
			return m, nil
		}
		m.loading = true
		m.errText = ""
		cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd { return bulkDetailsCmd(ctx, m.runtime, m.service, m.bulk.Plans) })
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
		return m.startNextBulkItem()
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
		return m, batchOpCmd(m.runtime.Context(), m.runtime, m.service, batchOpBulkCleanup, ids, m.torrents)
	case modeDelete:
		if len(m.deleteIDs) > 1 {
			m.loading = true
			ids := make([]string, len(m.deleteIDs))
			copy(ids, m.deleteIDs)
			return m, batchOpCmd(m.runtime.Context(), m.runtime, m.service, batchOpDelete, ids, m.torrents)
		}
		if len(m.deleteIDs) == 0 {
			m.mode = m.returnMode
			return m, nil
		}
		m.loading = true
		return m, deleteCmd(m.runtime.Context(), m.runtime, m.service, m.deleteIDs[0])
	case modeOverwrite:
		if m.pendingDownload == nil {
			m.mode = m.returnMode
			return m, nil
		}
		if m.pendingDownload.Queue != nil {
			m.mode = m.pendingDownload.ResumeMode
			if m.bulk == nil || !m.bulk.isRunning() || *m.pendingDownload.Queue != m.queueIdentity() {
				m.pendingDownload = nil
				return m, nil
			}
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
		cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd { return handoffCmd(ctx, m.runtime, m.service, targets[0]) })
		return *m, cmd
	}
	m.targets = targetPickerState{Items: targets, Action: action}
	m.mode = modeChooseTarget
	return *m, nil
}

func (m Model) beginBulkDownloadSetup() (tea.Model, tea.Cmd) {
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
		return bulkFileSelectionDetailsCmd(ctx, m.runtime, m.service, m.bulkSelect.Plans)
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
	return m, bulkFileSelectionSubmitCmd(m.runtime.Context(), m.runtime, m.service, submissions, m.bulkSelect.Outcomes)
}

func (m Model) startNextBulkItem() (tea.Model, tea.Cmd) {
	m.setup.invalidate()
	m.downloadPoll.invalidate()
	if m.bulk == nil {
		return m, nil
	}
	foreground := m.mode == modeBulkDownload || m.mode == modeBulkConfirm
	item := m.bulk.startNextItem()
	if item == nil {
		m.loading = false
		m.download = nil
		if foreground {
			m.mode = modeBulkDownload
		}
		success, failed, skipped := m.bulk.resultCounts()
		m.status = "Bulk download complete: " + bulkSummaryLine(success, failed, skipped)
		m.errText = ""
		return m, nil
	}
	if foreground {
		m.returnMode = modeBulkDownload
		m.mode = modeBulkDownload
	}
	m.download = nil
	m.loading = true
	m.status = fmt.Sprintf("Bulk downloading %d/%d: %s", m.bulk.Current+1, len(m.bulk.Items), bulkItemLabel(*item))
	m.errText = ""
	cmd := m.resolveDownload(item.Target)
	return m, cmd
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

func bootstrapCmd(ctx context.Context, runtime *app.Runtime, service app.AppService) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		session, err := service.Bootstrap(ctx)
		return bootstrapMsg{session: session, err: err}
	})
}

func authTokenCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, token string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		session, err := service.AuthenticateWithToken(ctx, token, true)
		return authMsg{session: session, err: err}
	})
}

func startDeviceCmd(ctx context.Context, runtime *app.Runtime, service app.AppService) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		code, err := service.StartDeviceFlow(ctx)
		return deviceStartMsg{code: code, err: err}
	})
}

func pollDeviceCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, code models.DeviceCode) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		session, err := service.CompleteDeviceFlow(ctx, code)
		return devicePollMsg{session: session, err: err}
	})
}

func refreshCmd(ctx context.Context, runtime *app.Runtime, service app.AppService) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		torrents, err := service.ListTorrents(ctx)
		return torrentsMsg{torrents: torrents, err: err}
	})
}

func bulkDetailsCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, plans []bulkTorrentPlan) tea.Cmd {
	if len(plans) == 0 {
		return nil
	}
	ordered := slices.Clone(plans)
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		details := make([]models.TorrentInfo, 0, len(ordered))
		for _, plan := range ordered {
			if err := ctx.Err(); err != nil {
				return bulkDetailsMsg{err: err}
			}
			info, err := service.TorrentInfo(ctx, plan.ID)
			if err != nil {
				return bulkDetailsMsg{err: fmt.Errorf("load %s details: %w", plan.Name, err)}
			}
			details = append(details, info)
		}
		return bulkDetailsMsg{details: details}
	})
}

func addMagnetCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, magnet string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		result, err := service.AddMagnet(ctx, magnet)
		return addTorrentMsg{result: result, err: err, label: "magnet"}
	})
}

func addURLCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, remoteURL string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		result, err := service.AddTorrentURL(ctx, remoteURL)
		return addTorrentMsg{result: result, err: err, label: "remote torrent URL"}
	})
}

func importCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, paths []string) tea.Cmd {
	paths = slices.Clone(paths)
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return importMsg{result: app.NewBatchService(batchServiceAPI{service}, service, nil).Import(ctx, paths)}
	})
}

func selectFilesCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, torrentID string, ids []int) tea.Cmd {
	ids = slices.Clone(ids)
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return selectFilesMsg{err: service.SelectFiles(ctx, torrentID, ids)}
	})
}

func deleteCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, torrentID string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return deleteMsg{err: service.DeleteTorrent(ctx, torrentID)}
	})
}

// Adapt the current facade without expanding every TUI dependency with raw API
// methods. Local imports remain compatible with demo and existing consumers.
type batchServiceAPI struct{ app.AppService }

func (s batchServiceAPI) UnrestrictLink(ctx context.Context, link string) (models.UnrestrictedLink, error) {
	return s.ResolveDirectURL(ctx, models.DownloadTarget{Link: link})
}

func (s batchServiceAPI) AddTorrentPath(ctx context.Context, path string) (models.AddTorrentResult, error) {
	results := s.ImportTorrentFiles(ctx, []string{path})
	if len(results) != 1 {
		return models.AddTorrentResult{}, errors.New("import returned no result")
	}
	return models.AddTorrentResult{ID: results[0].TorrentID}, results[0].Err
}

func batchOpCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, op batchOp, ids []string, torrents []models.Torrent) tea.Cmd {
	ids = slices.Clone(ids)
	torrents = slices.Clone(torrents)
	for idx := range torrents {
		torrents[idx].Links = slices.Clone(torrents[idx].Links)
	}
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		switch op {
		case batchOpDelete:
			return executeBatchDelete(ctx, service, ids)
		case batchOpBulkCleanup:
			result := executeBatchDelete(ctx, service, ids)
			result.op = batchOpBulkCleanup
			return result
		case batchOpCopy:
			return executeBatchCopy(ctx, service, ids, torrents)
		}
		return batchResultMsg{op: op, err: fmt.Errorf("unknown operation: %s", op)}
	})
}

func executeBatchDelete(ctx context.Context, service app.AppService, ids []string) batchResultMsg {
	batch := app.NewBatchService(batchServiceAPI{service}, service, nil).Delete(ctx, ids)
	return batchMessage(batchOpDelete, batch)
}

func executeBatchCopy(ctx context.Context, service app.AppService, ids []string, torrents []models.Torrent) batchResultMsg {
	byID := make(map[string]models.Torrent, len(torrents))
	for _, torrent := range torrents {
		byID[torrent.ID] = torrent
	}
	var requests []app.LinkRequest
	for _, id := range ids {
		torrent := byID[id]
		if torrent.Status != "downloaded" || len(torrent.Links) == 0 {
			requests = append(requests, app.LinkRequest{TorrentID: id})
			continue
		}
		for index, link := range torrent.Links {
			requests = append(requests, app.LinkRequest{TorrentID: id, Target: models.DownloadTarget{Index: index, Link: link}, Eligible: true})
		}
	}
	batch := app.NewBatchService(batchServiceAPI{service}, service, nil).CopyLinks(ctx, requests)
	result := batchMessage(batchOpCopy, batch.BatchResult)
	summary := app.SummarizeTorrents(batch.Items)
	result.total = summary.Success + summary.Partial + summary.Failed + summary.Skipped + summary.Canceled
	result.success, result.partial, result.failed, result.skipped, result.canceled = summary.Success, summary.Partial, summary.Failed, summary.Skipped, summary.Canceled
	result.resolved, result.copied = len(batch.ResolvedURLs), batch.CopiedURLs
	if batch.ClipboardErr != nil {
		result.detail += batch.ClipboardErr.Error() + "\n"
	}
	return result
}

func batchMessage(op batchOp, batch app.BatchResult) batchResultMsg {
	result := batchResultMsg{op: op, total: len(batch.Items), items: batch.Items, err: batch.Err}
	for _, item := range batch.Items {
		switch item.Kind {
		case app.OutcomeSuccess:
			result.success++
		case app.OutcomeFailed:
			result.failed++
		case app.OutcomeSkipped:
			result.skipped++
		case app.OutcomeCanceled:
			result.canceled++
		}
		if item.Err != nil {
			result.detail += fmt.Sprintf("%s: %v\n", item.TorrentID, item.Err)
		}
		if item.RemoteOutcomeUnknown {
			result.detail += "Remote outcome unknown; not replayed\n"
		}
	}
	return result
}

func handoffCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, target models.DownloadTarget) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		unrestricted, err := service.ResolveDirectURL(ctx, target)
		if err != nil {
			return handoffMsg{err: err}
		}
		url := unrestricted.Download
		if url == "" {
			url = unrestricted.Link
		}
		copied, err := service.CopyURL(url)
		return handoffMsg{result: models.HandoffResult{URL: url, Copied: copied}, err: err}
	})
}

func resolveDownloadCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, target models.DownloadTarget) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		unrestricted, err := service.ResolveDirectURL(ctx, target)
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
	})
}

func startManagedDownloadCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, url string, filename string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		result, err := service.Start(ctx, url, filename)
		return managedDownloadMsg{ref: result.Snapshot.Ref, result: models.ManagedDownloadStart{Download: result.Snapshot.Download, Reused: result.Reused}, err: err}
	})
}

func openDownloadCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, path string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return downloadPathMsg{action: "open", err: service.OpenFile(path)}
	})
}

func revealDownloadCmd(ctx context.Context, runtime *app.Runtime, service app.AppService, path string) tea.Cmd {
	return operationCmd(ctx, runtime, func(ctx context.Context) tea.Msg {
		return downloadPathMsg{action: "reveal", err: service.RevealInDirectory(path)}
	})
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
	cmd := m.beginFeature(setupFeature, func(ctx context.Context) tea.Cmd {
		return startManagedDownloadCmd(ctx, m.runtime, m.service, url, filename)
	})
	return *m, cmd
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
	normalized := strings.ReplaceAll(path, "\\", "/")
	if _, base, found := strings.CutLast(normalized, "/"); found {
		return base
	}
	return normalized
}
