package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/vmm/internal/ui"
)

const (
	minWidth  = 40
	minHeight = 14
)

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(ui.Cyan)

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(ui.Cyan)
	pointerStyle = lipgloss.NewStyle().Foreground(ui.Cyan)
	selectedRow  = lipgloss.NewStyle().Foreground(ui.Fg).Bold(true)
	dimRow       = lipgloss.NewStyle().Foreground(ui.Fg)
	autoMarker   = lipgloss.NewStyle().Foreground(ui.Magenta)

	keyStyle       = lipgloss.NewStyle().Foreground(ui.Magenta)
	footerStyle    = lipgloss.NewStyle().Foreground(ui.Comment)
	statusStyle    = lipgloss.NewStyle().Foreground(ui.Yellow)
	labelStyle     = lipgloss.NewStyle().Foreground(ui.Comment)
	sectionStyle   = lipgloss.NewStyle().Foreground(ui.Blue)
	detailLogStyle = lipgloss.NewStyle().Foreground(ui.Comment)
	valueStyle     = lipgloss.NewStyle().Foreground(ui.Fg)

	listBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
	previewBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
	hostBarBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ui.Comment).
			Padding(0, 1)
)

func glyph(it item) string {
	switch {
	case it.kind == kindHost:
		return lipgloss.NewStyle().Foreground(ui.Cyan).Render("◆")
	case it.running():
		return lipgloss.NewStyle().Foreground(ui.Green).Render("●")
	case it.status == "":
		return lipgloss.NewStyle().Foreground(ui.Yellow).Render("◎")
	default:
		return lipgloss.NewStyle().Foreground(ui.Comment).Render("○")
	}
}

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("terminal too small (min %d×%d)", minWidth, minHeight)
	}

	if m.mode == modeLogs {
		return m.renderLogs()
	}

	listW := m.width * 38 / 100
	if listW < 20 {
		listW = 20
	}
	previewW := m.width - listW - 6
	bodyH := m.height - 7
	if bodyH < 5 {
		bodyH = 5
	}

	list := listBorder.Width(listW).Height(bodyH).Render(clampBlock(m.renderList(), listW-4, bodyH))
	preview := previewBorder.Width(previewW).Height(bodyH).Render(m.renderPreview(previewW, bodyH))
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, preview)

	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderHostBar(), m.renderFooter())
}

func (m model) renderList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("vmm") + "\n\n")
	for i, it := range m.items {
		marker := "  "
		name := dimRow.Render(it.name)
		if i == m.cursor {
			marker = pointerStyle.Render("▌ ")
			name = selectedRow.Render(it.name)
		}
		trailing := ""
		if it.autostart {
			trailing += " " + autoMarker.Render("⏻")
		}
		if verb := m.tasks[it.name]; verb != "" {
			trailing += " " + m.spinner.View() + " " + labelStyle.Render(verb)
		}
		fmt.Fprintf(&b, "%s%s %s%s\n", marker, glyph(it), name, trailing)
	}
	return b.String()
}

func (m model) renderPreview(width, height int) string {
	cw := width - 4
	if cw < 10 {
		cw = 10
	}

	it := m.selected()
	if it.kind == kindHost {
		return clampBlock(m.renderHost(), cw, height)
	}
	creating := m.provisioning(it.name)

	var b strings.Builder
	statusWord := strings.ToLower(statusText(it))
	if creating {
		statusWord = statusStyle.Render("provisioning")
	}
	fmt.Fprintf(&b, "%s  %s %s\n\n", titleStyle.Render(it.name), glyph(it), statusWord)

	switch {
	case it.created():
		kv(&b, "cpus", it.inst.CPUs)
		kv(&b, "mem", ui.Bytes(it.inst.Memory))
		kv(&b, "disk", ui.Bytes(it.inst.Disk))
		if it.inst.SSHAddress != "" {
			kv(&b, "ssh", it.inst.SSHAddress+":"+it.inst.SSHLocalPort)
		}
		if it.inst.VMType != "" {
			kv(&b, "type", it.inst.VMType)
		}
		if it.inst.Arch != "" {
			kv(&b, "arch", it.inst.Arch)
		}
		if it.inst.Hostname != "" {
			kv(&b, "host", it.inst.Hostname)
		}
		kv(&b, "dir", it.inst.Dir)
		kv(&b, "auto", autostartLabel(it))
		if it.running() {
			b.WriteString("\n" + section("live", cw))
			m.renderGuest(&b, it.name)
		}
	case creating:
		kv(&b, "cpus", it.prof.Scalar("cpus"))
		kv(&b, "mem", it.prof.Scalar("memory"))
		kv(&b, "disk", it.prof.Scalar("disk"))
	default:
		b.WriteString(labelStyle.Render("not created, enter to provision") + "\n\n")
		kv(&b, "cpus", it.prof.Scalar("cpus"))
		kv(&b, "mem", it.prof.Scalar("memory"))
		kv(&b, "disk", it.prof.Scalar("disk"))
		kv(&b, "auto", autostartLabel(it))
		return clampBlock(b.String(), cw, height)
	}

	if m.logPeek != "" {

		used := strings.Count(b.String(), "\n")
		fit := height - used - 2
		if fit > 0 {
			lines := strings.Split(m.logPeek, "\n")
			if len(lines) > fit {
				lines = lines[len(lines)-fit:]
			}
			b.WriteString("\n" + section("logs", cw))
			for _, line := range lines {
				b.WriteString(detailLogStyle.Render(clip(line, cw)) + "\n")
			}
		}
	}
	return clampBlock(b.String(), cw, height)
}

func (m model) renderGuest(b *strings.Builder, name string) {
	e, ok := m.guest[name]
	if !ok {
		b.WriteString(labelStyle.Render("…") + "\n")
		return
	}
	if e.err {
		kv(b, "load", labelStyle.Render("unavailable"))
		return
	}
	s := e.stats
	kv(b, "load", s.Load)
	kv(b, "ram", fmt.Sprintf("%s / %s", ui.Bytes(u64(s.MemUsed)), ui.Bytes(u64(s.MemTotal))))
	kv(b, "used", fmt.Sprintf("%s / %s", ui.Bytes(u64(s.DiskUsed)), ui.Bytes(u64(s.DiskTotal))))
	if s.Uptime != "" {
		kv(b, "up", s.Uptime)
	}
}

func autostartLabel(it item) string {
	if it.autostart {
		return autoMarker.Render("⏻ on")
	}
	return "off"
}

func (m model) renderHost() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.host.name) + "\n\n")
	kv(&b, "os", m.host.os)
	kv(&b, "up", m.host.uptime)
	kv(&b, "shell", "local tmux session")
	return b.String()
}

func statusText(it item) string {
	if it.status == "" {
		return "not created"
	}
	return it.status
}

func kv(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s %s\n", labelStyle.Render(fmt.Sprintf("%-6s", key)), value)
}

func (m model) renderLogs() string {
	header := titleStyle.Render("logs: " + m.logName)
	if m.provisioning(m.logName) {
		header += " " + statusStyle.Render("provisioning…")
	}
	footer := keyStyle.Render("esc") + " " + footerStyle.Render("back")

	bodyH := m.height - 2
	if bodyH < 1 {
		bodyH = 1
	}
	lines := strings.Split(m.logView, "\n")
	if len(lines) > bodyH {
		lines = lines[len(lines)-bodyH:]
	}
	for i, line := range lines {
		lines[i] = detailLogStyle.Render(clip(line, m.width))
	}
	body := strings.Join(lines, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, truncate(header, m.width), body, footer)
}

func (m model) renderFooter() string {
	if m.mode == modeConfirmDelete {
		prompt := statusStyle.Render(fmt.Sprintf("delete %s? type the name: ", m.selected().name))
		return prompt + m.input.View() + "\n"
	}

	it := m.selected()
	var pairs [][2]string
	switch {
	case it.created():
		pairs = [][2]string{
			{"↵", "connect"}, {"^l", "logs"}, {"^s", "stop"},
			{"^r", "restart"}, {"^a", "auto"}, {"^x", "del"},
		}
	case m.provisioning(it.name):
		pairs = [][2]string{{"^l", "logs"}}
	case it.kind == kindVM:
		pairs = [][2]string{{"↵", "create"}}
	default:
		pairs = [][2]string{{"↵", "connect"}}
	}
	pairs = append(pairs, [2]string{"q", "quit"})

	var parts []string
	for _, p := range pairs {
		parts = append(parts, keyStyle.Render(p[0])+" "+footerStyle.Render(p[1]))
	}
	keys := truncate(strings.Join(parts, footerStyle.Render("  ")), m.width)
	return keys + "\n" + truncate(statusStyle.Render(m.status), m.width)
}

func (m model) renderHostBar() string {
	h := m.hostStats
	var running, vcpu int
	var committedMem uint64
	for _, it := range m.items {
		if !it.running() {
			continue
		}
		running++
		if n, err := strconv.Atoi(strings.TrimSpace(it.inst.CPUs)); err == nil {
			vcpu += n
		}
		if n, err := strconv.ParseUint(strings.TrimSpace(it.inst.Memory), 10, 64); err == nil {
			committedMem += n
		}
	}

	seg := func(label, value string) string {
		return labelStyle.Render(label+" ") + valueStyle.Render(value)
	}
	gap := footerStyle.Render("   ")

	left := strings.Join([]string{
		titleStyle.Render(m.host.name),
		seg("cores", strconv.Itoa(h.Cores)),
		seg("load", firstField(h.Load)),
		seg("ram", fmt.Sprintf("%s/%s", ui.Bytes(u64(h.MemUsed)), ui.Bytes(u64(h.MemTotal)))),
	}, gap)

	committed := seg("committed", fmt.Sprintf("%d vCPU / %s across %d running", vcpu, ui.Bytes(u64(committedMem)), running))

	content := truncate(left+gap+committed, m.width-4)
	return hostBarBorder.Width(m.width - 2).Render(content)
}

func section(title string, width int) string {
	head := sectionStyle.Render(title) + " "
	rule := width - lipgloss.Width(head)
	if rule < 0 {
		rule = 0
	}
	return head + footerStyle.Render(strings.Repeat("─", rule)) + "\n"
}

func clip(s string, width int) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, s)
	r := []rune(s)
	if len(r) > width {
		r = r[:width]
	}
	return string(r)
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

func u64(n uint64) string { return strconv.FormatUint(n, 10) }

func truncate(s string, width int) string {
	if width < 0 {
		width = 0
	}
	return ansi.Truncate(s, width, "")
}

func clampBlock(s string, width, height int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = truncate(line, width)
	}
	return strings.Join(lines, "\n")
}
