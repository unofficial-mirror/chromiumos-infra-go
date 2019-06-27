package osutils

import (
	"os"
	"path/filepath"
)

func abs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return abs
}

// Verify that the given relative path exists.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Verify that a directory exists at the given relative path.
func DirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}


// Look for a relative path, ascending through parent directories.
//
// Args:
//   path_to_find: The relative path to look for.
//   start_path: The path to start the search from.  If |start_path| is a
//   directory, it will be included in the directories that are searched.
//   end_path: The path to stop searching.
//   test_func: The function to use to verify the relative path.
func FindInPathParents(
	path_to_find string,
	start_path string,
	end_path string,
	test_func func(string) bool) string {

	// Default parameter values.
	if end_path == "" {
		end_path = "/"
	}
	if test_func == nil {
		test_func = PathExists
	}

	current_path := start_path
	for {
		// Test to see if path exists in this directory
		target_path := filepath.Join(abs(current_path), path_to_find)
		if test_func(target_path) {
			return abs(target_path)
		}

		rel, _ := filepath.Rel(end_path, current_path)
		if rel == "." {
			// Reached end_path.
			return ""
		}

		// Go up one directory.
		current_path += "/.."
	}
	return ""
}
