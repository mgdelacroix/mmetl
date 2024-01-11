package slack_grid

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const DefaultDirPath = "tmp/slack_grid"

func (t *BulkTransformer) GetWorkingDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "error getting current working directory")
	}
	return dir, nil
}

func (t *BulkTransformer) readDir(dest string) ([]fs.DirEntry, error) {
	files, err := os.ReadDir(dest)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error reading directory %v", dest))
	}
	return files, nil
}

func (t *BulkTransformer) dirHasContent(dest string) (bool, error) {

	err := os.MkdirAll(t.dirPath, os.ModePerm)
	if err != nil {
		return false, errors.Wrap(err, "error creating directory")
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		return false, errors.Wrap(err, "error reading directory")
	}

	if len(entries) > 0 {
		t.Logger.Errorf("directory %s is not empty. Using existing data.", dest)
		return true, nil
	}
	return false, nil
}

func (t *BulkTransformer) ExtractDirectory(zipReader *zip.Reader) error {
	t.Logger.Info("Extracting files...")
	pwd, err := t.GetWorkingDir()

	if err != nil {
		return errors.Errorf("error getting current working directory: %v", err)
	}
	t.pwd = pwd
	t.dirPath = filepath.Join(pwd, DefaultDirPath)
	t.Logger.Infof("Extracting to %s", t.dirPath)

	yes, err := t.dirHasContent(t.dirPath)
	if err != nil {
		return errors.Errorf("error seeing if directory has content already. %v", err)
	}

	if yes {
		t.Logger.Infof("content exists in the directory %s. Skipping extraction.", t.dirPath)
		return nil
	}

	totalFiles := len(zipReader.File)

	for i, f := range zipReader.File {
		fpath := filepath.Join(t.dirPath, f.Name)

		if f.FileInfo().IsDir() {
			// Make Folder
			err := os.MkdirAll(fpath, os.ModePerm)
			if err != nil {
				return errors.Wrap(err, "error creating directory")
			}
			continue
		}

		// Make File
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return errors.Wrap(err, "error creating directory")
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return errors.Wrap(err, "error creating files")
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return errors.Wrap(err, "error opening files")
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()
		if i%1000 == 0 || i == totalFiles-1 {
			t.Logger.Infof("Extracting file %d of %d", i, totalFiles)
		}
		if err != nil {
			return errors.Wrap(err, "error copying files")
		}
	}
	t.Logger.Info("Finished extracting files")

	return nil
}

func (t *BulkTransformer) ZipTeamDirectories() error {

	// zip the directories under /teams

	teams, err := t.readDir(filepath.Join(t.dirPath, "teams"))

	t.Logger.Infof("Zipping %v team directories...", len(teams))

	if err != nil {
		return errors.Wrap(err, "error reading teams directory")
	}
	// provide each as a root level export

	for _, team := range teams {
		teamPath := filepath.Join(t.dirPath, "teams", team.Name())
		teamZipPath := filepath.Join(t.pwd, team.Name()+".zip")

		t.Logger.Infof("Zipping %s to %s", teamPath, teamZipPath)

		err := ZipDir(teamPath, teamZipPath)
		if err != nil {
			return errors.Wrap(err, "error zipping team directory")
		}
	}
	// zip the remaining files up and provide a "leftovers" zip file.

	return nil
}

func ZipDir(source, target string) error {
	zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = filepath.Join(".", path[len(source):])

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			if err != nil {
				return err
			}
		}
		return err
	})

	return err
}