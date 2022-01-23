package corwhisk

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/ISE-SMILE/corral/compute/build"
	"github.com/apache/openwhisk-client-go/whisk"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	home, err := os.UserHomeDir()
	if err == nil {
		propsPath = filepath.Join(home, ".wskprops")
	} else {
		//best effort this will prop. work on unix and osx ;)
		propsPath = filepath.Join("~", ".wskprops")
	}
}

func readProps(in io.ReadCloser) map[string]string {
	defer in.Close()

	props := make(map[string]string)

	reader := bufio.NewScanner(in)

	for reader.Scan() {
		line := reader.Text()
		data := strings.SplitN(line, "=", 2)
		if len(data) < 2 {
			//This might leek user private data into a log...
			log.Errorf("could not read prop line %s", line)
		}
		props[data[0]] = data[1]
	}
	return props
}

func readEnviron() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		data := strings.SplitN(e, "=", 2)
		if len(data) < 2 {
			//This might leek user private data into a log...
			log.Errorf("could not read prop line %s", e)
		}
		env[data[0]] = data[1]
	}
	return env
}

func whiskClient(conf WhiskClientConfig) (*whisk.Client, error) {
	// lets first check the config
	host := conf.Host
	token := conf.Token
	var namespace = "_"

	if token == "" {
		//2. check if wskprops exsist
		if _, err := os.Stat(propsPath); err == nil {
			//. attempt to read and parse props
			if props, err := os.Open(propsPath); err == nil {
				host, token, namespace = setAuthFromProps(readProps(props))
			}
			//3. fallback try to check the env for token
		} else {
			host, token, namespace = setAuthFromProps(readEnviron())
		}
	}

	if token == "" {
		log.Warn("did not find a token for the whisk client!")
	}

	if conf.Namespace != "" {
		namespace = conf.Namespace
	}

	baseurl, _ := whisk.GetURLBase(host, "/api")
	clientConfig := &whisk.Config{
		Namespace:        namespace,
		AuthToken:        token,
		Host:             host,
		BaseURL:          baseurl,
		Version:          "v1",
		Verbose:          true,
		Insecure:         true,
		UserAgent:        "Golang/Smile cli",
		ApigwAccessToken: "Dummy Token",
	}

	client, err := whisk.NewClient(http.DefaultClient, clientConfig)
	if err != nil {
		return nil, err
	}

	err = ensurePlatformConfig(host, conf)
	if err != nil {
		return nil, err
	} else {
		return client, nil
	}
}

type OWInfoResponse struct {
	Features struct {
		BatchRequestFeature bool `json:"batch"`
		CallbackFeature     bool `json:"feedback"`
		LCHFeature          bool `json:"lch"`
	} `json:"features,omitempty"`
	Limits struct {
		ActionsPerMinute  int64 `json:"actions_per_minute"`
		ConcurrentActions int   `json:"concurrent_actions"`
		MaxActionDuration int64 `json:"max_action_duration"`
		MaxActionMemory   int64 `json:"max_action_memory"`
		MinActionDuration int64 `json:"min_action_duration"`
		MinActionMemory   int64 `json:"min_action_memory"`
	} `json:"limits,omitempty"`
}

//ensurePlatformConfig checks if openwhisk implements required plafrom features
func ensurePlatformConfig(host string, conf WhiskClientConfig) error {

	req, err := http.NewRequest(http.MethodGet, host, nil)
	if err != nil {
		return fmt.Errorf("could not verifiy platform %+v", err)
	}
	req.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not verifiy platform %+v", err)
	}
	if response.StatusCode == http.StatusOK {
		data, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("could not verifiy platform %+v", err)
		}
		var rw = OWInfoResponse{}
		err = json.Unmarshal(data, &rw)
		if err != nil {
			return fmt.Errorf("could not parese platform %+v info", err)
		}

		if conf.MultiDeploymentFeature || conf.BatchRequestFeature || conf.Address != nil {
			if rw.Features.BatchRequestFeature != conf.BatchRequestFeature {
				return fmt.Errorf("platform dose not support batch request feautre")
			}

			if conf.Address == nil && rw.Features.CallbackFeature {
				return fmt.Errorf("platform dose not support callback feature")
			}

			if rw.Features.LCHFeature != conf.DataPreloadingFeature {
				return fmt.Errorf("platform dose not support data preloading feature")
			}
		}

		if conf.RequestPerMinute > rw.Limits.ActionsPerMinute {
			return fmt.Errorf("plaform has lower request per minute then requried by config")
		}

		if conf.ConcurrencyLimit > rw.Limits.ConcurrentActions {
			return fmt.Errorf("platform concurrent limit lower then requried by config")
		}
	} else {
		return fmt.Errorf("platfrom verifcation response invalid")
	}

	return nil
}

//check props and env vars for relevant infomation ;)
func setAuthFromProps(auth map[string]string) (string, string, string) {
	var host string
	var token string
	var namespace string
	if apihost, ok := auth["APIHOST"]; ok {
		host = apihost
	} else if apihost, ok := auth["__OW_API_HOST"]; ok {
		host = apihost
	}
	if apitoken, ok := auth["AUTH"]; ok {
		token = apitoken
	} else if apitoken, ok := auth["__OW_API_KEY"]; ok {
		token = apitoken
	}
	if apinamespace, ok := auth["NAMESPACE"]; ok {
		namespace = apinamespace
	} else if apinamespace, ok := auth["__OW_NAMESPACE"]; ok {
		namespace = apinamespace
	}
	return host, token, namespace
}

func (l *WhiskClient) preparePayload(payload interface{}) WhiskPayload {
	invocation := WhiskPayload{
		Value: payload,
		Env:   make(map[string]*string),
	}

	build.InjectConfiguration(invocation.Env)
	strBool := "true"
	invocation.Env["DEBUG"] = &strBool
	if l.remoteLoggingHost != "" {
		invocation.Env["RemoteLoggingHost"] = &l.remoteLoggingHost
	}
	return invocation
}

// returns name and namespace based on function name
func GetQualifiedName(functioname string) (string, string) {
	var namespace string
	var action string

	if strings.HasPrefix(functioname, "/") {
		parts := strings.Split(functioname, "/")
		namespace = parts[0]
		action = parts[1]
	} else {
		//no namespace set in string using _
		namespace = "_"
		action = functioname
	}

	return action, namespace

}
