// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	cycler_pb "go.chromium.org/chromiumos/infra/proto/go/cycler"

	"cloud.google.com/go/storage"
	"github.com/golang/glog"
	"golang.org/x/sync/semaphore"
)

var newline = fmt.Sprintf("\n")

// Runlog is a sink for jsonl logs produced by cycler. jsonl is simply
// individual json objects without newlines each placed on a single line
//
// It writes in approximate chunks, and can run over size. There are some
// interesting ways use of this logger can affect performance depending on
// how many logs are outstanding, response times from GS\Disk etc. Generally
// things happen in coroutines unless you've fallen way behind and then it
// will revert to a blocking mode. This may  become an issue, but we don't
// want to either prematurely optimize or allow unbounded unshipped logs.
//
// Logs will pre appended with a timestamp and the uniq id of the cycler run.
//
// It currently allows both gs:// URLs as well as local file:// urls.
type Runlog struct {

	// LogSink is the channel where producers can push their logs.
	LogSink chan []byte

	// Config is the RunLogConfiguration
	Config cycler_pb.RunLogConfiguration

	// Stop channel signals the logging routines to stop.
	Stop chan bool

	// logBuffer is an internal array of bytes to buffer incoming messages to.
	logBuffer [][]byte

	// logBufferSize is the size of the log buffer.
	logBufferSize int64

	// client is the google storage client.
	client *storage.Client

	// destURL is the gs:// or file:// url desitnation for the logs.
	dstURL *url.URL

	// wg is the waitgroup for routines of the logger.
	wg *sync.WaitGroup

	// logShippers counts the number of routines currently engaged in shipping logs.
	logShippers *semaphore.Weighted
}

// Init sets up the runlog, with json config bytes or nil if defaults should be used.
func (rl *Runlog) Init(config cycler_pb.RunLogConfiguration, client *storage.Client, wg *sync.WaitGroup) {
	ctx := context.Background()

	// Initialize channels, waitgroups and semaphores.
	rl.Config = config
	rl.Stop = make(chan bool, 1)
	rl.logBuffer = make([][]byte, 0)
	rl.LogSink = make(chan []byte, rl.Config.ChannelSize)
	rl.client = client
	rl.wg = wg
	rl.logShippers = semaphore.NewWeighted(rl.Config.MaxUnpersistedLogs)

	// Parse / Validate the destination URL.
	dstURL, err := url.Parse(rl.Config.DestinationUrl)
	if err != nil {
		glog.Errorf("DestinationURL failed to parse runlog config: %v\n", err)
		os.Exit(2)
	}
	rl.dstURL = dstURL

	// Sanity test location writable by clobbering onto a special file.
	tstMsg := "Written to validate logging location writablility, ignore please."

	// Path has a leading / and we omit it.
	tstPath := path.Join(rl.dstURL.Path, "ignore-sanity-test")

	switch scheme := rl.dstURL.Scheme; scheme {
	case "gs":
		bkt := rl.client.Bucket(dstURL.Host)
		_, err = bkt.Attrs(ctx)
		if err != nil {
			glog.Errorf("error writing logs, bucket couldn't be retrieved: %v", err)
			os.Exit(2)
		}
		obj := bkt.Object(tstPath)
		f := obj.NewWriter(ctx)
		_, err := f.Write([]byte(tstMsg))
		if err != nil {
			glog.Errorf("error writing logs, write failed: %v", err)
			os.Exit(2)
		}
		f.Close()

	case "file":
		err = os.MkdirAll(path.Dir(tstPath), os.ModePerm)
		if err != nil {
			glog.Errorf("log directory apparently uncreatable: %v\n", err)
			os.Exit(2)
		}
		f, err := os.Create(tstPath)
		if err != nil {
			glog.Errorf("log directory apparently unwritable: %v\n", err)
			os.Exit(2)
		}
		_, err = f.WriteString(tstMsg)
		if err != nil {
			glog.Errorf("log directory apparently unwritable: %v\n", err)
			os.Exit(2)
		}
		// Best effort delete.
		os.Remove(tstPath)
	default:
		glog.Errorf("unexpected log destination uri (accepts file:// and gs://")
		os.Exit(2)
	}

	rl.wg.Add(1)
	go rl.loggingCoordinator()
}

func (rl *Runlog) loggingCoordinator() {
	defer func() {
		if r := recover(); r != nil {
			glog.Errorf("will not recover from panic in loggingCoordinator as we will _not_ run unlogged: %v", r)
			os.Exit(2)
		}
		// Flag your exit for waiters.
		rl.wg.Done()
	}()

	for {
		select {

		// New log incoming.
		case log := <-rl.LogSink:
			incomingSize := int64(len(log))
			if rl.logBufferSize+incomingSize >= rl.Config.ChunkSizeBytes {
				rl.flush()
			}
			rl.logBufferSize += incomingSize
			rl.logBuffer = append(rl.logBuffer, log)

		// Stop signal given.
		case <-rl.Stop:
			// Put all outstanding logs on rl.LogSink (expand to fit).
			for i := 0; i < len(rl.LogSink); i++ {
				incomingLog := <-rl.LogSink
				rl.logBufferSize += int64(len(incomingLog))
				rl.logBuffer = append(rl.logBuffer, incomingLog)
			}

			// Flush.
			rl.flush()

			// Collect on your log shippers by trying to gobble entire semaphore capacity.
			var sleepTime time.Duration = 2 * time.Second
			var n int64
			for n = 0; n < rl.Config.PersistRetries; n++ {
				if rl.logShippers.TryAcquire(rl.Config.MaxUnpersistedLogs) {
					return
				}
				glog.Infof("Logs are still being persisted, waiting %v...", sleepTime)
				time.Sleep(sleepTime)
				sleepTime <<= 1
			}
			glog.Error("Returned with oustanding unpersisted logs!")
		}
	}
}

func (rl *Runlog) flush() error {
	ctx := context.Background()

	// No logs, no flush.
	if rl.logBufferSize == 0 {
		return nil
	}

	// Combine messages into a single byte[] called data and 1 byte per newline.
	dataSize := rl.logBufferSize + int64(len(rl.logBuffer))
	data := make([]byte, dataSize)
	nWritten := 0
	for i := 0; i < len(rl.logBuffer); i++ {
		llen := len(rl.logBuffer[i])
		copy(data[nWritten:], rl.logBuffer[i])
		nWritten += llen
		copy(data[nWritten:], newline)
		nWritten++
	}

	// data is transfered to the goroutine, do not access after this point.
	rl.logShippers.Acquire(ctx, 1)
	go rl.compressAndPersistLog(&data)

	// Clear rl.logBuffer & reset logBufferSize.
	rl.logBuffer = make([][]byte, 0)
	rl.logBufferSize = 0
	return nil
}

// Compress some data and persist it. Called as go routine. Retries persist.
// This will be executed as a coroutine and should only access rl for configuration.
func (rl Runlog) compressAndPersistLog(data *[]byte) {
	defer func() {
		rl.logShippers.Release(1)
	}()

	compressedBytes, err := compressBytes(data)
	if err != nil {
		glog.Errorf("log upload failed on compression, not likely to succeed on retry, aborting.")
		// TODO(engeg@) reevaluate if we want to end the process here. We certainly
		// want our logs to be persisted...
		os.Exit(1)
	}

	var sleepTime time.Duration = 2 * time.Second
	var n int64
	for n = 0; n < rl.Config.PersistRetries; n++ {
		if err := rl.persistLog(context.Background(), compressedBytes); err != nil {
			glog.V(0).Infof("retrying failed upload %v: %v\nsleeping for %v...", n, err, sleepTime)
			time.Sleep(sleepTime)
			sleepTime <<= 1
		} else {
			// We're persisted :)
			break
		}
	}
}

// persistLog handles writing/uploading the log buffer whos ownership was given to it.
// This will be executed as a coroutine and should only access rl for configuration.
func (rl Runlog) persistLog(ctx context.Context, compressedBytes *bytes.Buffer) error {
	logName := rl.createLogName()

	switch scheme := rl.dstURL.Scheme; scheme {
	case "gs":
		// Path has a leading / and we omit it.
		gspath := path.Join(rl.dstURL.Path[1:], logName)
		obj := rl.client.Bucket(rl.dstURL.Host).Object(gspath)
		f := obj.NewWriter(ctx)
		n, err := f.Write(compressedBytes.Bytes())
		if n != compressedBytes.Len() || err != nil {
			glog.Errorf("error writing logs, write failed: %v", err)
			return err
		}
		f.Close()
		glog.V(2).Infof("log uploaded to: gs://%v/%v", rl.dstURL.Host, gspath)

	case "file":
		// Attempt to create log dir up to file.
		fullPath := path.Join(rl.dstURL.Path, logName)
		err := os.MkdirAll(path.Dir(fullPath), os.ModePerm)
		if err != nil {
			glog.Errorf("log directory apparently uncreatable: %v\n", err)
			return err
		}
		f, err := os.Create(fullPath)
		if err != nil {
			glog.Errorf("error writing logs, couldn't open file: %v", err)
			return err
		}

		n, err := f.Write(compressedBytes.Bytes())
		if n != compressedBytes.Len() || err != nil {
			glog.Errorf("error writing logs, write failed: %v", err)
			return err
		}
		f.Close()
		glog.V(1).Infof("log written to: %v", fullPath)
	}

	return nil
}

// Generate a timestamped unique name for each log. Time is log written time.
func (rl Runlog) createLogName() string {
	timePart := time.Now().UTC().Format(time.RFC3339Nano)
	return fmt.Sprintf("%v/%v.jsonl.gz", cyclerInvocationID, timePart)
}
