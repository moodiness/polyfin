package config

import "os"

// writableByAnother tells no fault on Windows: a folder has neither an
// owner nor a mode there that os.FileInfo tells, and the folders of a
// user's profile, the default data folder among them, are kept from the
// other accounts by its access control list.
func writableByAnother(string, os.FileInfo, int) *DataFolderFault {
	return nil
}
