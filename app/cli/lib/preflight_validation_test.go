package lib

import (
	"os"
	"path/filepath"
	"testing"

	"plandex-cli/fs"
)

func TestCheckProjectRootWritable(t *testing.T) {
	orig := fs.ProjectRoot
	defer func() { fs.ProjectRoot = orig }()

	t.Run("writable_dir_passes", func(t *testing.T) {
		tmp := t.TempDir()
		fs.ProjectRoot = tmp
		if ve := checkProjectRootWritable(); ve != nil {
			t.Errorf("expected nil, got: %s", ve.Message)
		}
	})

	t.Run("missing_dir_fails", func(t *testing.T) {
		fs.ProjectRoot = "/nonexistent_path_abc123"
		ve := checkProjectRootWritable()
		if ve == nil {
			t.Fatal("expected error for missing directory")
		}
		if ve.Code != "PROJECT_ROOT_MISSING" {
			t.Errorf("expected PROJECT_ROOT_MISSING, got %s", ve.Code)
		}
	})

	t.Run("empty_path_fails", func(t *testing.T) {
		fs.ProjectRoot = ""
		ve := checkProjectRootWritable()
		if ve == nil {
			t.Fatal("expected error for empty path")
		}
		if ve.Code != "PROJECT_ROOT_UNSET" {
			t.Errorf("expected PROJECT_ROOT_UNSET, got %s", ve.Code)
		}
	})

	t.Run("file_not_dir_fails", func(t *testing.T) {
		tmp := t.TempDir()
		filePath := filepath.Join(tmp, "notadir")
		os.WriteFile(filePath, []byte("x"), 0644)
		fs.ProjectRoot = filePath
		ve := checkProjectRootWritable()
		if ve == nil {
			t.Fatal("expected error for non-directory path")
		}
		if ve.Code != "PROJECT_ROOT_NOT_DIR" {
			t.Errorf("expected PROJECT_ROOT_NOT_DIR, got %s", ve.Code)
		}
	})
}

func TestCheckShellAvailable(t *testing.T) {
	origShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", origShell)

	t.Run("shell_set_passes", func(t *testing.T) {
		os.Setenv("SHELL", "/bin/bash")
		if ve := checkShellAvailable(); ve != nil {
			t.Errorf("expected nil when SHELL is set, got: %s", ve.Message)
		}
	})

	t.Run("shell_unset_bash_exists_passes", func(t *testing.T) {
		os.Setenv("SHELL", "")
		// On most systems /bin/bash exists; if not, skip.
		if _, err := os.Stat("/bin/bash"); err != nil {
			t.Skip("/bin/bash not available on this system")
		}
		if ve := checkShellAvailable(); ve != nil {
			t.Errorf("expected nil when /bin/bash exists, got: %s", ve.Message)
		}
	})

	t.Run("shell_unset_no_bash_fails", func(t *testing.T) {
		os.Setenv("SHELL", "")
		// We can't actually remove /bin/bash, so test the logic by
		// temporarily replacing the check's fallback path via a
		// separate helper. Instead, just verify the function structure.
		// Full integration test requires a sandboxed env without /bin/bash.
		t.Skip("requires environment without /bin/bash")
	})
}

func TestCheckReplOutputDir(t *testing.T) {
	origVal := os.Getenv("PLANDEX_REPL_OUTPUT_FILE")
	defer os.Setenv("PLANDEX_REPL_OUTPUT_FILE", origVal)

	t.Run("unset_passes", func(t *testing.T) {
		os.Setenv("PLANDEX_REPL_OUTPUT_FILE", "")
		if ve := checkReplOutputDir(); ve != nil {
			t.Errorf("expected nil when env unset, got: %s", ve.Message)
		}
	})

	t.Run("valid_parent_passes", func(t *testing.T) {
		tmp := t.TempDir()
		os.Setenv("PLANDEX_REPL_OUTPUT_FILE", filepath.Join(tmp, "output.txt"))
		if ve := checkReplOutputDir(); ve != nil {
			t.Errorf("expected nil for valid parent dir, got: %s", ve.Message)
		}
	})

	t.Run("missing_parent_fails", func(t *testing.T) {
		os.Setenv("PLANDEX_REPL_OUTPUT_FILE", "/nonexistent_dir_xyz/output.txt")
		ve := checkReplOutputDir()
		if ve == nil {
			t.Fatal("expected error for missing parent directory")
		}
		if ve.Code != "REPL_OUTPUT_DIR_MISSING" {
			t.Errorf("expected REPL_OUTPUT_DIR_MISSING, got %s", ve.Code)
		}
	})
}

func TestCheckAPIHostValid(t *testing.T) {
	origVal := os.Getenv("PLANDEX_API_HOST")
	defer os.Setenv("PLANDEX_API_HOST", origVal)

	t.Run("unset_passes", func(t *testing.T) {
		os.Setenv("PLANDEX_API_HOST", "")
		if ve := checkAPIHostValid(); ve != nil {
			t.Errorf("expected nil when env unset, got: %s", ve.Message)
		}
	})

	t.Run("valid_url_passes", func(t *testing.T) {
		os.Setenv("PLANDEX_API_HOST", "https://api.example.com")
		if ve := checkAPIHostValid(); ve != nil {
			t.Errorf("expected nil for valid URL, got: %s", ve.Message)
		}
	})

	t.Run("invalid_url_fails", func(t *testing.T) {
		os.Setenv("PLANDEX_API_HOST", "not-a-url")
		ve := checkAPIHostValid()
		if ve == nil {
			t.Fatal("expected error for invalid URL")
		}
		if ve.Code != "API_HOST_INVALID" {
			t.Errorf("expected API_HOST_INVALID, got %s", ve.Code)
		}
	})

	t.Run("scheme_only_fails", func(t *testing.T) {
		os.Setenv("PLANDEX_API_HOST", "https://")
		ve := checkAPIHostValid()
		if ve == nil {
			t.Fatal("expected error for scheme-only URL")
		}
		if ve.Code != "API_HOST_INVALID" {
			t.Errorf("expected API_HOST_INVALID, got %s", ve.Code)
		}
	})
}

func TestCheckProjectsFileValid(t *testing.T) {
	origHome := fs.HomePlandexDir
	defer func() { fs.HomePlandexDir = origHome }()

	t.Run("missing_file_passes", func(t *testing.T) {
		fs.HomePlandexDir = t.TempDir() // empty dir — file doesn't exist
		if ve := checkProjectsFileValid(); ve != nil {
			t.Errorf("expected nil for missing file, got: %s", ve.Message)
		}
	})

	t.Run("valid_json_passes", func(t *testing.T) {
		tmp := t.TempDir()
		fs.HomePlandexDir = tmp
		os.WriteFile(filepath.Join(tmp, "projects-v2.json"), []byte(`[{"id":"p1"}]`), 0644)
		if ve := checkProjectsFileValid(); ve != nil {
			t.Errorf("expected nil for valid JSON, got: %s", ve.Message)
		}
	})

	t.Run("malformed_json_fails", func(t *testing.T) {
		tmp := t.TempDir()
		fs.HomePlandexDir = tmp
		os.WriteFile(filepath.Join(tmp, "projects-v2.json"), []byte(`{broken`), 0644)
		ve := checkProjectsFileValid()
		if ve == nil {
			t.Fatal("expected error for malformed JSON")
		}
		if ve.Code != "PROJECTS_FILE_MALFORMED" {
			t.Errorf("expected PROJECTS_FILE_MALFORMED, got %s", ve.Code)
		}
	})

	t.Run("empty_file_warns", func(t *testing.T) {
		tmp := t.TempDir()
		fs.HomePlandexDir = tmp
		os.WriteFile(filepath.Join(tmp, "projects-v2.json"), []byte(""), 0644)
		ve := checkProjectsFileValid()
		if ve == nil {
			t.Fatal("expected warning for empty file")
		}
		if ve.Code != "PROJECTS_FILE_EMPTY" {
			t.Errorf("expected PROJECTS_FILE_EMPTY, got %s", ve.Code)
		}
	})
}

func TestCheckSettingsFileValid(t *testing.T) {
	origHome := fs.HomePlandexDir
	defer func() { fs.HomePlandexDir = origHome }()

	t.Run("missing_file_passes", func(t *testing.T) {
		fs.HomePlandexDir = t.TempDir()
		if ve := checkSettingsFileValid(); ve != nil {
			t.Errorf("expected nil for missing file, got: %s", ve.Message)
		}
	})

	t.Run("valid_json_passes", func(t *testing.T) {
		tmp := t.TempDir()
		fs.HomePlandexDir = tmp
		os.WriteFile(filepath.Join(tmp, "settings-v2.json"), []byte(`{"theme":"dark"}`), 0644)
		if ve := checkSettingsFileValid(); ve != nil {
			t.Errorf("expected nil for valid JSON, got: %s", ve.Message)
		}
	})

	t.Run("malformed_json_fails", func(t *testing.T) {
		tmp := t.TempDir()
		fs.HomePlandexDir = tmp
		os.WriteFile(filepath.Join(tmp, "settings-v2.json"), []byte(`]invalid[`), 0644)
		ve := checkSettingsFileValid()
		if ve == nil {
			t.Fatal("expected error for malformed JSON")
		}
		if ve.Code != "SETTINGS_FILE_MALFORMED" {
			t.Errorf("expected SETTINGS_FILE_MALFORMED, got %s", ve.Code)
		}
	})

	t.Run("empty_file_warns", func(t *testing.T) {
		tmp := t.TempDir()
		fs.HomePlandexDir = tmp
		os.WriteFile(filepath.Join(tmp, "settings-v2.json"), []byte(""), 0644)
		ve := checkSettingsFileValid()
		if ve == nil {
			t.Fatal("expected warning for empty file")
		}
		if ve.Code != "SETTINGS_FILE_EMPTY" {
			t.Errorf("expected SETTINGS_FILE_EMPTY, got %s", ve.Code)
		}
	})
}
