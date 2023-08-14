package data_integrity

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/mattermost/mattermost-server/v6/app/imports"
	"github.com/mattermost/mattermost-server/v6/model"
)

type SyncImportUsersFlags struct {
	UpdateUsers bool
	OutputFile  string
}

func SyncImportUsers(reader io.Reader, flags SyncImportUsersFlags, client *model.Client4, logger *logrus.Logger) error {
	scanner := bufio.NewScanner(reader)

	var out *os.File
	var err error

	var writeLine = func(line string) error {
		if out != nil {
			if _, writeErr := out.WriteString(line + "\n"); writeErr != nil {
				return errors.Wrap(writeErr, "Failed to write to output file")
			}
		}

		return nil
	}

	if flags.UpdateUsers {
		out, err = os.OpenFile(flags.OutputFile, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return errors.Wrap(err, "Error opening output file")
		}
		defer out.Close()
	}

	foundUser := false
	usersChanged := []string{}

	logger.Info("Starting sync process")
	for scanner.Scan() {
		var lineData imports.LineImportData

		line := scanner.Text()
		err := json.Unmarshal([]byte(line), &lineData)
		if err != nil {
			logger.Warnf("Error unmarshalling line, continuing process anyway: %v", err)
			if writeErr := writeLine(line); writeErr != nil {
				return writeErr
			}
		}

		if lineData.Type != "user" {
			if foundUser && !flags.UpdateUsers {
				break
			}

			if writeErr := writeLine(line); writeErr != nil {
				return writeErr
			}
			continue
		}

		foundUser = true

		user := lineData.User
		logger.Debugf("Processing user %s", *user.Username)

		recordChanged, err := mergeImportFileUser(user, flags, client, logger)
		if err != nil {
			logger.Errorf("Error checking user %s, keeping import record as-is. %v", *user.Username, err)
			continue
		}

		if recordChanged {
			usersChanged = append(usersChanged, *user.Username)
		}

		userOut, err := json.Marshal(user)
		if err != nil {
			return errors.Wrap(err, "Error marshaling user")
		}

		if writeErr := writeLine(string(userOut)); writeErr != nil {
			return writeErr
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if !flags.UpdateUsers {
		logger.Info("Exited after reading users from import file, due to not providing --update-users flag")
	}

	logger.Infof("Number of users with changes: %d", len(usersChanged))
	if len(usersChanged) > 0 {
		logger.Infof("Users changed: %s", strings.Join(usersChanged, ", "))
	}

	logger.Info("Finished sync process")

	return nil
}

func mergeImportFileUser(user *imports.UserImportData, flags SyncImportUsersFlags, client *model.Client4, logger *logrus.Logger) (bool, error) {
	usernameExists := false
	emailExists := false

	emailFromImport := strings.ToLower(*user.Email)
	usernameFromImport := strings.ToLower(*user.Username)

	existingUserByUsername, resp, err := client.GetUserByUsername(usernameFromImport, "")
	if err != nil {
		if resp.StatusCode != 404 {
			return false, err
		}

		logger.Debugf("Username %s does not exist in database", usernameFromImport)
	} else {
		usernameExists = true
		logger.Debugf("Username %s exists in database", usernameFromImport)
	}

	existingUserByEmail, resp, err := client.GetUserByEmail(emailFromImport, "")
	if err != nil {
		if resp.StatusCode != 404 {
			return false, err
		}

		logger.Debugf("Email %s does not exist in database", emailFromImport)
	} else {
		emailExists = true
		logger.Debugf("Email %s exists in database", emailFromImport)
	}

	if usernameExists && existingUserByUsername.Email != emailFromImport {
		logger.Warnf("Username %s already exists, but has a different email. DB email: (%s) Import file email (%s)", usernameFromImport, existingUserByUsername.Email, emailFromImport)
	}

	if emailExists && existingUserByEmail.Username != usernameFromImport {
		logger.Warnf("Email %s already exists, but has a different username. DB username: (%s) Import file username (%s)", emailFromImport, existingUserByEmail.Username, usernameFromImport)
	}

	recordChanged := false
	if usernameExists && existingUserByUsername.Email != emailFromImport {
		recordChanged = true
		user.Email = &existingUserByUsername.Email
		if flags.UpdateUsers {
			logger.Infof("Updating email for user %s from %s to %s", usernameFromImport, emailFromImport, existingUserByUsername.Email)
			recordChanged = true
		} else {
			logger.Infof("Use the `--update-users` flag to update the import file's user record for user %s", usernameFromImport)
		}
	}

	if emailExists && existingUserByEmail.Username != usernameFromImport {
		recordChanged = true
		user.Username = &existingUserByEmail.Username
		if flags.UpdateUsers {
			logger.Infof("Updating username for user %s from %s to %s", emailFromImport, usernameFromImport, existingUserByEmail.Username)
		} else {
			logger.Infof("Use the `--update-users` flag to update the import file's user record for user %s", usernameFromImport)
		}
	}

	if !recordChanged {
		logger.Debugf("Record not changed for user %s", usernameFromImport)
	}

	return recordChanged, nil

}
