package tui

import (
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/niklod/kin/internal/daemon"
)

// --- Message types ---

type statusResult struct {
	resp *daemon.StatusResponse
	err  error
}

type peersResult struct {
	resp *daemon.PeersResponse
	err  error
}

type catalogResult struct {
	resp *daemon.CatalogResponse
	err  error
}

type inviteResult struct {
	resp *daemon.InviteResponse
	err  error
}

type joinResult struct {
	resp *daemon.JoinResponse
	err  error
}

type daemonEvent struct{ daemon.Event }

type daemonDisconnected struct{}

// --- Command factories ---

func fetchStatus(client DaemonClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Status()
		return statusResult{resp: resp, err: err}
	}
}

func fetchPeers(client DaemonClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Peers()
		return peersResult{resp: resp, err: err}
	}
}

func fetchCatalog(client DaemonClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Catalog()
		return catalogResult{resp: resp, err: err}
	}
}

func doInvite(client DaemonClient) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Invite()
		return inviteResult{resp: resp, err: err}
	}
}

func doJoin(client DaemonClient, token string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Join(token)
		return joinResult{resp: resp, err: err}
	}
}

type downloadResult struct {
	fileID string
	err    error
}

func doDownload(client DaemonClient, fileID string) tea.Cmd {
	return func() tea.Msg {
		_, err := client.Download(fileID)
		return downloadResult{fileID: fileID, err: err}
	}
}

func waitForEvent(ch <-chan daemon.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return daemonDisconnected{}
		}
		return daemonEvent{ev}
	}
}

// decodePeerInfo decodes a PeerInfo from event data.
func decodePeerInfo(data json.RawMessage) (daemon.PeerInfo, bool) {
	var info daemon.PeerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return info, false
	}
	return info, true
}

// decodePeerOffline decodes a PeerOfflineEvent from event data.
func decodePeerOffline(data json.RawMessage) (daemon.PeerOfflineEvent, bool) {
	var ev daemon.PeerOfflineEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return ev, false
	}
	return ev, true
}
