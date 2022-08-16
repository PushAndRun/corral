package polling

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type PredictionRequest struct {
}

type Response struct {
	Prediction [][]float64 `json:"predictions"`
}

type Request struct {
	signature string    `json:"signature_name"`
	Instances []float64 `json:"instances"`
}

func (p *PredictionRequest) GetPrediction(apiKey string, feat []float64) (int, error) {
	url := "http://localhost:8501/v1/models/" + apiKey + ":predict"
	var message Request
	message.Instances = feat
	request_obj, jsonErr := json.Marshal(message)
	if jsonErr != nil {
		log.Fatal(jsonErr)
		return 0, jsonErr
	}

	resp, getErr := http.Post(url, "application/json", bytes.NewBuffer(request_obj))
	if getErr != nil {
		log.Fatal(getErr)
		return 0, getErr
	}

	if resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, readErr := ioutil.ReadAll(resp.Body)
		if readErr != nil {
			log.Fatal(readErr)
			return 0, readErr
		}

		response := Response{}
		unmErr := json.Unmarshal(body, &response)
		if unmErr != nil {
			log.Fatal(unmErr)
			return 0, unmErr
		}

		log.Debug("Received prediction: ", response.Prediction[0][0])
		return int(response.Prediction[0][0]), nil
	} else {
		log.Error("Prediction server status code " + fmt.Sprint(resp.StatusCode))
		return 0, errors.New("Predection server unavailable or misconfigured")
	}
}
