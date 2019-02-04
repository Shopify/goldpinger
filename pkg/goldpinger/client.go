// Copyright 2018 Bloomberg Finance L.P.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package goldpinger

import (
	"fmt"
	"sync"
	"time"

	apiclient "github.com/bloomberg/goldpinger/pkg/client"
	"github.com/bloomberg/goldpinger/pkg/models"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

// CheckNeighbours queries the kubernetes API server for all other goldpinger pods
// then calls Ping() on each one
func CheckNeighbours() models.CheckResults {
	return PingAllPods(GetAllPods())
}

// CheckNeighboursNeighbours queries the kubernetes API server for all other goldpinger
// pods then calls Check() on each one
func CheckNeighboursNeighbours() *models.CheckAllResults {
	return CheckAllPods(GetAllPods())
}

type PingAllPodsResult struct {
	podResult models.PodResult
	hostIPv4  strfmt.IPv4
	podIP     string
}

func pickPodHostIP(podIP, hostIP string) string {
	if GoldpingerConfig.UseHostIP {
		return hostIP
	}
	return podIP
}

func PingAllPods(pods map[string]string) models.CheckResults {

	result := models.CheckResults{}

	ch := make(chan PingAllPodsResult, len(pods))
	wg := sync.WaitGroup{}
	wg.Add(len(pods))

	for podIP, hostIP := range pods {

		go func(podIP string, hostIP string) {
			var channelResult PingAllPodsResult

			CountCall("made", "ping")
			timer := GetLabeledPeersCallsTimer("ping", hostIP, podIP)
			start := time.Now()
			resp, err := getClient(pickPodHostIP(podIP, hostIP)).Operations.Ping(nil)

			channelResult.hostIPv4.UnmarshalText([]byte(hostIP))
			var OK = (err == nil)
			if OK {
				channelResult.podResult = models.PodResult{
					HostIP:          channelResult.hostIPv4,
					OK:              &OK,
					Response:        resp.Payload,
					StatusCode:      200,
					RoundTripTimeMS: int32(time.Since(start) / time.Millisecond),
				}
				timer.ObserveDuration()
			} else {
				channelResult.podResult = models.PodResult{
					HostIP:     channelResult.hostIPv4,
					OK:         &OK,
					Error:      err.Error(),
					StatusCode: 500,
				}
				CountError("ping")
			}
			channelResult.podIP = podIP

			ch <- channelResult
			wg.Done()
		}(podIP, hostIP)
	}
	wg.Wait()
	close(ch)

	counterHealthy, counterUnhealthy := 0.0, 0.0

	for response := range ch {
		var podIPv4 strfmt.IPv4
		podIPv4.UnmarshalText([]byte(response.podIP))
		if *response.podResult.OK {
			counterHealthy++
		} else {
			counterUnhealthy++
		}
		result[response.podIP] = response.podResult
	}
	CountHealthyUnhealthyNodes(counterHealthy, counterUnhealthy)
	return result
}

type CheckServicePodsResult struct {
	checkAllPodResult models.CheckAllPodResult
	hostIPv4          strfmt.IPv4
	podIP             string
}

func CheckAllPods(pods map[string]string) *models.CheckAllResults {

	result := models.CheckAllResults{Responses: make(map[string]models.CheckAllPodResult)}

	ch := make(chan CheckServicePodsResult, len(pods))
	wg := sync.WaitGroup{}
	wg.Add(len(pods))

	for podIP, hostIP := range pods {

		go func(podIP string, hostIP string) {
			var channelResult CheckServicePodsResult

			CountCall("made", "check")
			timer := GetLabeledPeersCallsTimer("check", hostIP, podIP)
			resp, err := getClient(pickPodHostIP(podIP, hostIP)).Operations.CheckServicePods(nil)

			channelResult.hostIPv4.UnmarshalText([]byte(hostIP))
			var OK = (err == nil)
			if OK {
				channelResult.checkAllPodResult = models.CheckAllPodResult{
					OK:       &OK,
					HostIP:   channelResult.hostIPv4,
					Response: resp.Payload,
				}
				timer.ObserveDuration()
			} else {
				channelResult.checkAllPodResult = models.CheckAllPodResult{
					OK:     &OK,
					HostIP: channelResult.hostIPv4,
					Error:  err.Error(),
				}
				CountError("checkAll")
			}
			channelResult.podIP = podIP

			ch <- channelResult
			wg.Done()
		}(podIP, hostIP)
	}
	wg.Wait()
	close(ch)

	for response := range ch {
		var podIPv4 strfmt.IPv4
		podIPv4.UnmarshalText([]byte(response.podIP))

		result.Responses[response.podIP] = response.checkAllPodResult
		result.Hosts = append(result.Hosts, &models.CheckAllResultsHostsItems0{
			HostIP: response.hostIPv4,
			PodIP:  podIPv4,
		})
	}
	return &result
}

func HealthCheck() *models.HealthCheckResults {
	ok := true
	start := time.Now()
	result := models.HealthCheckResults{
		OK:          &ok,
		DurationNs:  time.Since(start).Nanoseconds(),
		GeneratedAt: strfmt.DateTime(start),
	}
	return &result
}

func getClient(hostIP string) *apiclient.Goldpinger {
	host := fmt.Sprintf("%s:%d", hostIP, GoldpingerConfig.Port)
	transport := httptransport.New(host, "", nil)
	client := apiclient.New(transport, strfmt.Default)
	apiclient.Default.SetTransport(transport)

	return client
}
