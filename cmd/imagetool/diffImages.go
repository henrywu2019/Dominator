package main

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	imgclient "github.com/Symantec/Dominator/imageserver/client"
	"github.com/Symantec/Dominator/lib/constants"
	"github.com/Symantec/Dominator/lib/filesystem"
	"github.com/Symantec/Dominator/lib/image"
	"github.com/Symantec/Dominator/lib/srpc"
	"github.com/Symantec/Dominator/proto/sub"
	subclient "github.com/Symantec/Dominator/sub/client"
)

func diffSubcommand(args []string) {
	diffTypedImages(args[0], args[1], args[2])
}

func diffTypedImages(tool string, lName string, rName string) {
	lfs, err := getTypedImage(lName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting left image: %s\n", err)
		os.Exit(1)
	}
	if lfs, err = applyDeleteFilter(lfs); err != nil {
		fmt.Fprintf(os.Stderr, "Error filtering left image: %s\n", err)
		os.Exit(1)
	}
	rfs, err := getTypedImage(rName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting right image: %s\n", err)
		os.Exit(1)
	}
	if rfs, err = applyDeleteFilter(rfs); err != nil {
		fmt.Fprintf(os.Stderr, "Error filtering right image: %s\n", err)
		os.Exit(1)
	}
	err = diffImages(tool, lfs, rfs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error diffing images: %s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func getTypedImage(typedName string) (*filesystem.FileSystem, error) {
	if len(typedName) < 3 || typedName[1] != ':' {
		imageSClient, _ := getClients()
		return getFsOfImage(imageSClient, typedName)
	}
	switch name := typedName[2:]; typedName[0] {
	case 'd':
		return scanDirectory(name)
	case 'f':
		return readFileSystem(name)
	case 'i':
		imageSClient, _ := getClients()
		return getFsOfImage(imageSClient, name)
	case 'l':
		return readFsOfImage(name)
	case 's':
		return pollImage(name)
	default:
		return nil, errors.New("unknown image type: " + typedName[:1])
	}
}

func scanDirectory(name string) (*filesystem.FileSystem, error) {
	fs, err := buildImageWithHasher(nil, nil, name, nil)
	if err != nil {
		return nil, err
	}
	return fs, nil
}

func readFileSystem(name string) (*filesystem.FileSystem, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var fileSystem filesystem.FileSystem
	if err := gob.NewDecoder(file).Decode(&fileSystem); err != nil {
		return nil, err
	}
	fileSystem.RebuildInodePointers()
	return &fileSystem, nil
}

func getImage(client *srpc.Client, name string) (*image.Image, error) {
	img, err := imgclient.GetImageWithTimeout(client, name, *timeout)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, errors.New(name + ": not found")
	}
	img.FileSystem.RebuildInodePointers()
	return img, nil
}

func getFsOfImage(client *srpc.Client, name string) (
	*filesystem.FileSystem, error) {
	if image, err := getImage(client, name); err != nil {
		return nil, err
	} else {
		return image.FileSystem, nil
	}
}

func readFsOfImage(name string) (*filesystem.FileSystem, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var image image.Image
	if err := gob.NewDecoder(file).Decode(&image); err != nil {
		return nil, err
	}
	image.FileSystem.RebuildInodePointers()
	return image.FileSystem, nil
}

func pollImage(name string) (*filesystem.FileSystem, error) {
	clientName := fmt.Sprintf("%s:%d", name, constants.SubPortNumber)
	srpcClient, err := srpc.DialHTTP("tcp", clientName, 0)
	if err != nil {
		return nil, fmt.Errorf("Error dialing %s", err)
	}
	defer srpcClient.Close()
	var request sub.PollRequest
	var reply sub.PollResponse
	if err = subclient.CallPoll(srpcClient, request, &reply); err != nil {
		return nil, err
	}
	if reply.FileSystem == nil {
		return nil, errors.New("no poll data")
	}
	reply.FileSystem.RebuildInodePointers()
	return reply.FileSystem, nil
}

func diffImages(tool string, lfs, rfs *filesystem.FileSystem) error {
	lname, err := writeImage(lfs)
	defer os.Remove(lname)
	if err != nil {
		return err
	}
	rname, err := writeImage(rfs)
	defer os.Remove(rname)
	if err != nil {
		return err
	}
	cmd := exec.Command(tool, lname, rname)
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func writeImage(fs *filesystem.FileSystem) (string, error) {
	file, err := ioutil.TempFile("", "imagetool")
	if err != nil {
		return "", err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	return file.Name(), fs.Listf(writer, listSelector, listFilter)
}
