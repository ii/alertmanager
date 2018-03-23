package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

type getResponse struct {
	Status    string        `json:"status"`
	Data      types.Silence `json:"data,omitempty"`
	ErrorType string        `json:"errorType,omitempty"`
	Error     string        `json:"error,omitempty"`
}

var (
	updateCmd      = silenceCmd.Command("update", "Update silences")
	updateDuration = updateCmd.Flag("duration", "Duration of silence").Short('d').String()
	updateStart    = updateCmd.Flag("start", "Set when the silence should start. RFC3339 format 2006-01-02T15:04:05Z07:00").String()
	updateEnd      = updateCmd.Flag("end", "Set when the silence should end (overwrites duration). RFC3339 format 2006-01-02T15:04:05Z07:00").String()
	updateComment  = updateCmd.Flag("comment", "A comment to help describe the silence").Short('c').String()
	updateIds      = updateCmd.Arg("update-ids", "Silence IDs to update").Strings()
)

func init() {
	updateCmd.Action(update)
	longHelpText["silence update"] = `Extend or update existing silence in Alertmanager.`
}

func update(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	if len(*updateIds) < 1 {
		return fmt.Errorf("no silence IDs specified")
	}

	alertmanagerUrl := GetAlertmanagerURL("/api/v1/silence")
	updatedSilenceIDs := make([]string, 0, len(*updateIds))
	for _, silenceId := range *updateIds {
		silence, err := getSilenceById(silenceId, alertmanagerUrl)
		if err != nil {
			return err
		}
		newSilenceID, err := updateSilence(silence)
		if err != nil {
			return err
		}
		updatedSilenceIDs = append(updatedSilenceIDs, newSilenceID)
	}

	for _, id := range updatedSilenceIDs {
		fmt.Println(id)
	}
	return nil
}

// This takes an url.URL and not a pointer as we will modify it for our API call.
func getSilenceById(silenceId string, baseUrl url.URL) (*types.Silence, error) {
	baseUrl.Path = path.Join(baseUrl.Path, silenceId)
	res, err := http.Get(baseUrl.String())
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("couldn't read response body: %v", err)
	}

	if res.StatusCode == 404 {
		return nil, fmt.Errorf("no silence found with id: %v", silenceId)
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("received %d response from Alertmanager: %v", res.StatusCode, body)
	}

	var response getResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode response body: %v", err)
	}
	return &response.Data, nil
}

func updateSilence(silence *types.Silence) (string, error) {
	var err error
	if *updateEnd != "" {
		silence.EndsAt, err = time.Parse(time.RFC3339, *updateEnd)
		if err != nil {
			return "", err
		}
	} else if *updateDuration != "" {
		d, err := model.ParseDuration(*updateDuration)
		if err != nil {
			return "", err
		}
		if d == 0 {
			return "", fmt.Errorf("silence duration must be greater than 0")
		}
		silence.EndsAt = silence.EndsAt.UTC().Add(time.Duration(d))
	}

	if *updateStart != "" {
		silence.StartsAt, err = time.Parse(time.RFC3339, *updateStart)
		if err != nil {
			return "", err
		}
	}

	if silence.StartsAt.After(silence.EndsAt) {
		return "", errors.New("silence cannot start after it ends")
	}

	if *updateComment != "" {
		silence.Comment = *updateComment
	}

	// addSilence can also be used to update an existing silence
	newSilenceID, err := addSilence(silence)
	if err != nil {
		return "", err
	}
	return newSilenceID, nil
}
