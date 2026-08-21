package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/liteploy/liteploy/internal/docker"
)

// handleDeploymentSSE streams deployment log events via Server-Sent Events.
//
// SSE protocol: each event is "data: <line>\n\n"
// The client (HTMX + htmx-sse extension) opens this endpoint when on the
// deployment detail page and receives log lines as they are written.
//
// Bounded: we stop streaming once the deployment reaches a terminal state.
// We never store the full log in RAM — we tail the file.
func (s *Server) handleDeploymentSSE(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dep, err := s.depSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable Nginx buffering

	// Extend write deadline for SSE.
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(30 * time.Minute))

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial status event.
	fmt.Fprintf(w, "event: status\ndata: {\"status\":%q}\n\n", dep.Status)
	flusher.Flush()

	// If already terminal, send existing log and close.
	if dep.Status.IsTerminal() {
		s.streamLogSSE(w, flusher, dep.ID)
		fmt.Fprintf(w, "event: done\ndata: {\"status\":%q}\n\n", dep.Status)
		flusher.Flush()
		return
	}

	// Poll-and-stream while the deployment is active.
	// We avoid aggressive 1-second polling by using a 2-second ticker with
	// early exit once the deployment becomes terminal.
	// This is acceptable for a 1 GB VPS where SSE connections are few.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var logOffset int64

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Stream new log lines since last poll.
			newOffset, err := s.streamLogSSEFrom(w, flusher, dep.ID, logOffset)
			if err == nil {
				logOffset = newOffset
			}

			// Check deployment status.
			latest, err := s.depSvc.Get(id)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "event: status\ndata: {\"status\":%q,\"stage\":%q}\n\n", latest.Status, latest.Stage)
			flusher.Flush()

			if latest.Status.IsTerminal() {
				fmt.Fprintf(w, "event: done\ndata: {\"status\":%q}\n\n", latest.Status)
				flusher.Flush()
				return
			}
		}
	}
}

// streamLogSSE streams the full build log to the SSE connection.
func (s *Server) streamLogSSE(w io.Writer, flusher http.Flusher, deploymentID string) {
	s.streamLogSSEFrom(w, flusher, deploymentID, 0)
}

// streamLogSSEFrom streams log lines starting from offset, returns new offset.
func (s *Server) streamLogSSEFrom(w io.Writer, flusher http.Flusher, deploymentID string, offset int64) (int64, error) {
	dep, err := s.depSvc.Get(deploymentID)
	if err != nil {
		return offset, err
	}
	_ = dep

	// Use a pipe writer to capture log lines and send as SSE events.
	pr, pw := io.Pipe()
	go func() {
		err := s.depSvc.StreamBuildLog(deploymentID, pw)
		pw.CloseWithError(err)
	}()
	defer pr.Close()

	buf := make([]byte, 4096)
	newOffset := offset

	// Skip to offset.
	skipped := int64(0)
	for skipped < offset {
		toSkip := int64(len(buf))
		if toSkip > offset-skipped {
			toSkip = offset - skipped
		}
		n, err := pr.Read(buf[:toSkip])
		skipped += int64(n)
		if err != nil {
			break
		}
	}

	// Stream remaining lines.
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			line := string(buf[:n])
			fmt.Fprintf(w, "data: %s\n\n", line)
			newOffset += int64(n)
			flusher.Flush()
		}
		if err != nil {
			break
		}
	}

	return newOffset, nil
}

// handleContainerLogStream streams container stdout/stderr in real-time.
// This is an SSE endpoint for viewing live container output.
func (s *Server) handleContainerLogStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := s.appSvc.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if s.dockerCli == nil {
		http.Error(w, "Docker service unavailable", http.StatusServiceUnavailable)
		return
	}

	if app.ContainerID == "" {
		http.Error(w, "no container found for application", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(30 * time.Minute))

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	pr, pw := io.Pipe()
	go func() {
		err := s.dockerCli.StreamLogs(r.Context(), app.ContainerID, docker.LogOptions{
			Follow:     true,
			Tail:       "100",
			Timestamps: true,
		}, pw)
		pw.CloseWithError(err)
	}()
	defer pr.Close()

	buf := make([]byte, 4096)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			line := string(buf[:n])
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
		if err != nil {
			break
		}
	}
}
