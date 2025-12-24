package pod

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
)

// LogsPanel displays container logs with streaming support
type LogsPanel struct {
	root    *tview.Flex
	laidout bool

	// Sub-panels
	infoPanel   *tview.Flex
	logsView    *tview.TextView
	footerPanel *tview.Flex

	// Log state
	namespace     string
	podName       string
	containerName string
	following     bool
	timestamps    bool
	wrapText      bool
	tailLines     int64
	lineCount     int

	// Streaming control
	cancelFunc context.CancelFunc
	streamMu   sync.Mutex

	// Callbacks
	onBack       func()
	getLogStream func(ctx context.Context, namespace, podName string, opts k8s.LogOptions) (io.ReadCloser, error)
	queueUpdate  func(func())
}

// NewLogsPanel creates a new container logs panel
func NewLogsPanel() *LogsPanel {
	p := &LogsPanel{
		root:      tview.NewFlex().SetDirection(tview.FlexRow),
		following: true,  // Default to following
		wrapText:  true,  // Default to wrapped
		tailLines: 1000,  // Default tail lines
	}

	p.setupInputCapture()
	return p
}

// SetOnBack sets the callback for when user wants to go back
func (p *LogsPanel) SetOnBack(callback func()) {
	p.onBack = callback
}

// SetLogStreamFunc sets the function to get log streams
func (p *LogsPanel) SetLogStreamFunc(fn func(ctx context.Context, namespace, podName string, opts k8s.LogOptions) (io.ReadCloser, error)) {
	p.getLogStream = fn
}

// SetQueueUpdateFunc sets the function for queuing UI updates
func (p *LogsPanel) SetQueueUpdateFunc(fn func(func())) {
	p.queueUpdate = fn
}

// GetRootView returns the root view for this panel
func (p *LogsPanel) GetRootView() tview.Primitive {
	return p.root
}

// ShowLogs displays logs for the specified container
func (p *LogsPanel) ShowLogs(namespace, podName, containerName string) {
	p.streamMu.Lock()

	// Cancel any existing stream
	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}

	p.namespace = namespace
	p.podName = podName
	p.containerName = containerName
	p.lineCount = 0

	p.streamMu.Unlock()

	// Build UI if not laid out
	if !p.laidout {
		p.buildLayout()
		p.laidout = true
	}

	// Update info panel
	p.updateInfoPanel()

	// Clear logs view
	p.logsView.Clear()

	// Start streaming logs
	p.startLogStream(false)
}

func (p *LogsPanel) buildLayout() {
	// Info panel (3 rows)
	p.infoPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.infoPanel.SetBorder(true).SetTitle(fmt.Sprintf(" %s Container Logs ", ui.Icons.Info))

	// Logs view (flex)
	p.logsView = tview.NewTextView()
	p.logsView.SetDynamicColors(true)
	p.logsView.SetScrollable(true)
	p.logsView.SetWrap(p.wrapText)
	p.logsView.SetBorder(true)

	// Footer panel (2 rows) with keyboard shortcuts
	p.footerPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.footerPanel.SetBorder(false)

	// Build footer with shortcuts
	p.buildFooter()

	// Assemble layout
	p.root.Clear()
	p.root.AddItem(p.infoPanel, 3, 0, false)
	p.root.AddItem(p.logsView, 0, 1, true)
	p.root.AddItem(p.footerPanel, 2, 0, false)
}

func (p *LogsPanel) updateInfoPanel() {
	p.infoPanel.Clear()

	// Container info
	containerInfo := tview.NewTextView()
	containerInfo.SetDynamicColors(true)
	containerInfo.SetText(fmt.Sprintf("[yellow]Container:[white] %s", p.containerName))

	// Pod info
	podInfo := tview.NewTextView()
	podInfo.SetDynamicColors(true)
	podInfo.SetText(fmt.Sprintf("[yellow]Pod:[white] %s/%s", p.namespace, p.podName))

	// Status info
	statusInfo := tview.NewTextView()
	statusInfo.SetDynamicColors(true)
	followStatus := "[gray]PAUSED"
	if p.following {
		followStatus = "[green]FOLLOWING"
	}
	statusInfo.SetText(fmt.Sprintf("[yellow]Status:[white] %s  [yellow]Lines:[white] %d", followStatus, p.lineCount))

	p.infoPanel.AddItem(containerInfo, 0, 1, false)
	p.infoPanel.AddItem(podInfo, 0, 1, false)
	p.infoPanel.AddItem(statusInfo, 0, 1, false)
}

func (p *LogsPanel) buildFooter() {
	p.footerPanel.Clear()

	shortcuts := []struct {
		key  string
		desc string
	}{
		{"f", "follow"},
		{"p", "previous"},
		{"t", "timestamps"},
		{"w", "wrap"},
		{"g/G", "top/bottom"},
		{"ESC", "back"},
	}

	for _, s := range shortcuts {
		item := tview.NewTextView()
		item.SetDynamicColors(true)
		item.SetTextAlign(tview.AlignCenter)
		item.SetText(fmt.Sprintf("[yellow]%s[white] %s", s.key, s.desc))
		p.footerPanel.AddItem(item, 0, 1, false)
	}
}

func (p *LogsPanel) setupInputCapture() {
	p.root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			p.stopStream()
			if p.onBack != nil {
				p.onBack()
			}
			return nil

		case tcell.KeyRune:
			switch event.Rune() {
			case 'f', 'F':
				p.toggleFollow()
				return nil
			case 'p', 'P':
				p.showPreviousLogs()
				return nil
			case 't', 'T':
				p.toggleTimestamps()
				return nil
			case 'w', 'W':
				p.toggleWrap()
				return nil
			case 'g':
				p.logsView.ScrollToBeginning()
				return nil
			case 'G':
				p.logsView.ScrollToEnd()
				return nil
			}
		}
		return event
	})
}

func (p *LogsPanel) toggleFollow() {
	p.streamMu.Lock()
	p.following = !p.following
	p.streamMu.Unlock()

	if p.following {
		// Restart stream in follow mode
		p.startLogStream(false)
	} else {
		// Stop streaming
		p.stopStream()
	}

	p.updateInfoPanel()
	p.queueRedraw()
}

func (p *LogsPanel) showPreviousLogs() {
	p.stopStream()
	p.logsView.Clear()
	p.lineCount = 0
	p.startLogStream(true)
}

func (p *LogsPanel) toggleTimestamps() {
	p.streamMu.Lock()
	p.timestamps = !p.timestamps
	p.streamMu.Unlock()

	// Restart stream with new settings
	p.stopStream()
	p.logsView.Clear()
	p.lineCount = 0
	p.startLogStream(false)
}

func (p *LogsPanel) toggleWrap() {
	p.wrapText = !p.wrapText
	p.logsView.SetWrap(p.wrapText)
	p.queueRedraw()
}

func (p *LogsPanel) startLogStream(previous bool) {
	if p.getLogStream == nil {
		p.appendLog("[red]Error: log stream function not configured")
		return
	}

	p.streamMu.Lock()
	// Cancel any existing stream
	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancelFunc = cancel

	opts := k8s.LogOptions{
		Container:  p.containerName,
		Follow:     p.following && !previous,
		Previous:   previous,
		Timestamps: p.timestamps,
		TailLines:  p.tailLines,
	}

	namespace := p.namespace
	podName := p.podName
	p.streamMu.Unlock()

	go func() {
		stream, err := p.getLogStream(ctx, namespace, podName, opts)
		if err != nil {
			p.appendLog(fmt.Sprintf("[red]Error getting logs: %v", err))
			return
		}
		defer stream.Close()

		scanner := bufio.NewScanner(stream)
		// Increase buffer size for long lines
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := scanner.Text()
				p.appendLog(p.formatLogLine(line))
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case <-ctx.Done():
				// Context cancelled, don't report error
			default:
				p.appendLog(fmt.Sprintf("[yellow]Stream ended: %v", err))
			}
		}
	}()
}

func (p *LogsPanel) stopStream() {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()

	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}
}

func (p *LogsPanel) formatLogLine(line string) string {
	// Color timestamps if present (RFC3339 format: 2024-01-15T10:30:00Z)
	if p.timestamps && len(line) > 30 && line[4] == '-' && line[7] == '-' {
		// Try to find the timestamp separator
		if idx := strings.Index(line, " "); idx > 0 && idx < 35 {
			timestamp := line[:idx]
			rest := line[idx+1:]
			return fmt.Sprintf("[gray]%s[white] %s", timestamp, rest)
		}
	}

	// Color error/warning lines
	lineLower := strings.ToLower(line)
	if strings.Contains(lineLower, "error") || strings.Contains(lineLower, "fatal") {
		return "[red]" + line
	}
	if strings.Contains(lineLower, "warn") {
		return "[yellow]" + line
	}

	return line
}

func (p *LogsPanel) appendLog(line string) {
	p.streamMu.Lock()
	p.lineCount++
	count := p.lineCount
	following := p.following
	p.streamMu.Unlock()

	if p.queueUpdate != nil {
		p.queueUpdate(func() {
			fmt.Fprintln(p.logsView, line)
			if following {
				p.logsView.ScrollToEnd()
			}
			// Update line count in info panel periodically
			if count%100 == 0 {
				p.updateInfoPanel()
			}
		})
	}
}

func (p *LogsPanel) queueRedraw() {
	if p.queueUpdate != nil {
		p.queueUpdate(func() {
			// Just trigger a redraw
		})
	}
}

// Cleanup stops any active streams
func (p *LogsPanel) Cleanup() {
	p.stopStream()
}

// SetFollowing sets the follow mode
func (p *LogsPanel) SetFollowing(follow bool) {
	p.streamMu.Lock()
	p.following = follow
	p.streamMu.Unlock()
}

// SetTailLines sets the number of tail lines to fetch
func (p *LogsPanel) SetTailLines(lines int64) {
	p.tailLines = lines
}

// Focus returns focus handling hint
func (p *LogsPanel) Focus() {
	// LogsView handles focus
}

// RefreshStatus updates the status display
func (p *LogsPanel) RefreshStatus() {
	p.updateInfoPanel()
	p.queueRedraw()
}

// GetLogTimestamp returns time since logs started
func (p *LogsPanel) GetLogTimestamp() time.Duration {
	return 0 // Could track start time if needed
}
