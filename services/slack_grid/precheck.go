package slack_grid

import (
	"archive/zip"
)

func (t *BulkTransformer) GridPreCheck(zipReader *zip.Reader) bool {

	requiredFiles := []string{
		// "org_users.json",
		"channels.json",
		"dms.json",
		"groups.json",
		"mpims.json",
	}

	valid := true

	for _, fileName := range requiredFiles {
		fileExists := t.Transformer.CheckForRequiredFile(zipReader, fileName)
		valid = valid && fileExists
	}

	if len(t.Teams) == 0 {
		t.Logger.Error("no teams found in teams.json")
		valid = false
	}

	return valid
}
