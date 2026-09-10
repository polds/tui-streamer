package server

import (
	"log"

	"github.com/polds/tui-streamer/internal/bundle"
	"github.com/polds/tui-streamer/internal/executor"
	"github.com/polds/tui-streamer/internal/session"
)

// ApplyFileConfig merges allowlists and theme settings from a parsed bundle
// into the live server config. TUI_PATH is not changed here — files can only
// be staged from an on-disk bundle path (startup -bundle or a packaged .app).
func (s *Server) ApplyFileConfig(f *bundle.File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.AllowedCommands = bundle.MergeAllowlists(s.cfg.AllowedCommands, f.Allow)
	if f.Theme != "" {
		s.cfg.Theme = f.Theme
	}
	if len(f.Themes) > 0 {
		s.cfg.Themes = bundle.FilterThemes(f.Themes)
	}
}

// ImportFile creates sessions declared in f and autoruns eligible commands.
func (s *Server) ImportFile(f *bundle.File) {
	s.ApplyFileConfig(f)
	for _, b := range f.Bundles {
		log.Printf("bundle %q: loading %d session(s)", b.Name, len(b.Sessions))
		for _, entry := range b.Sessions {
			sess := s.manager.Create(entry.Name, b.Name)
			sess.PendingCommand = entry.Command
			sess.Description = entry.Description
			if entry.Autorun && entry.Command != "" {
				s.autoExec(sess, entry)
			} else {
				log.Printf("bundle: created %q (manual execution)", entry.Name)
			}
		}
	}
}

func (s *Server) autoExec(sess *session.Session, entry bundle.Entry) {
	words, err := executor.SplitCommand(entry.Command)
	if err != nil {
		log.Printf("bundle: skip auto-exec %q: invalid command: %v", entry.Name, err)
		return
	}
	opts := s.execOptions(words)
	if !bundle.CommandAllowed(s.allowed(), opts.Command) {
		log.Printf("bundle: skip auto-exec %q: command not allowed", entry.Name)
		return
	}
	if err := sess.Exec(opts); err != nil {
		log.Printf("bundle: auto-exec %q: %v", entry.Name, err)
		return
	}
	log.Printf("bundle: auto-exec %q: started", entry.Name)
}

func (s *Server) execOptions(command []string) executor.Options {
	s.mu.RLock()
	defer s.mu.RUnlock()
	command = executor.ExpandTUIPath(command, s.cfg.TUIPath)
	return executor.Options{
		Command: command,
		Dir:     s.cfg.Dir,
		Stdout:  s.cfg.Stdout,
		Stderr:  s.cfg.Stderr,
		TUIPath: s.cfg.TUIPath,
	}
}

func (s *Server) allowed() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.cfg.AllowedCommands) == 0 {
		return nil
	}
	out := make([]string, len(s.cfg.AllowedCommands))
	copy(out, s.cfg.AllowedCommands)
	return out
}

func (s *Server) themeConfig() (defaultTheme string, themes []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	allowed := bundle.FilterThemes(s.cfg.Themes)
	return bundle.ResolveDefaultTheme(s.cfg.Theme, allowed), allowed
}
