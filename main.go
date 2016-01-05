package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
	"os"
	"log"
	"sync"
	"strconv"
	"crypto/md5"
)

type Location struct {
	Latitude float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

//Status int constants
const pending int = 1
const confirmed int = 2
const completed int = 3
const inactive int = 0

type Pickup struct {
	PhoneNumber string `json:"phoneNumber"`
	InitialLocation Location `json:"initialLocation"`
	InitialTime time.Time `json:"initialTime"`
	LatestLocation Location `json:"latestLocation"`
	LatestTime time.Time `json:"latestTime"`
	Status int `json:"status"`
	ConfirmTime time.Time `json:"confirmTime"`
	CompleteTime time.Time `json:"completeTime"`
}

var pickups map[string]Pickup

var vanLocations []Location

var startTime = time.Now()

var successResponse string
var failResponse string

func generateSuccessResponse(targetString *string) {
	tmp, err := json.Marshal(map[string]string{"status":"success"})
	*targetString = string(tmp)
	if  err != nil {
		fmt.Printf("Generating success response failed. %v", err)
	}
}

func generateFailResponse(targetString *string) {
	tmp, err := json.Marshal(map[string]string{"status":"fail"})
	*targetString = string(tmp)
	if  err != nil {
		fmt.Printf("Generating fail response failed. %v", err)
	}
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	http.Redirect(w, r, "https://github.com/ansonl/shipmate", http.StatusFound)
}

func doKeysExist(targetDictionary url.Values, targetKeys []string) bool {
	for _, v := range targetKeys {
		if _, exists := targetDictionary[v]; !exists {
			return false
		}
	}
	return true
}

func areFieldsEmpty(targetDictionary url.Values, targetKeys []string) bool {
	for _, v := range targetKeys {
		if len(targetDictionary[v]) == 0 {
			return true
		}
	}
	return false
}

func isFieldEmpty(field string) bool {
	if len(field) > 0 {
		return false
	}
	return true
}

func checkMD5(password []byte) bool {
	digest := fmt.Sprintf("%x", md5.Sum(password))
	if digest == "34d1f8a7e29f3f3497ec05d0c9c8e4fc" {
		return true
	}
	return false
}

func isPhraseCorrect(targetDictionary url.Values) bool {
	if doKeysExist(targetDictionary, []string{"phrase"}) && !areFieldsEmpty(targetDictionary ,[]string{"phrase"}) {
		if checkMD5([]byte(targetDictionary["phrase"][0])) {
			return true
		} else {
			fmt.Println("Wrong phrase \"" + targetDictionary["phrase"][0] + "\" received")
		}
	} else {
		fmt.Println("No phrase HTTP parameter received.")
	}
	return false
}

func uptimeHandler(w http.ResponseWriter, r *http.Request) {
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	diff := time.Since(startTime)

	fmt.Fprintf(w, "" + "Uptime:\t" + diff.String())
	fmt.Println("Uptime requested")
}

func newPickup(w http.ResponseWriter, r *http.Request) {
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	r.ParseForm()

	if !doKeysExist(r.Form, []string{"phoneNumber", "latitude", "longitude"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber", "latitude", "longitude"}) {
		log.Fatal("required http parameters not found for newPickup")
	}

	var number string
	var location Location

	/*
	number, err := strconv.Atoi(r.Form["phoneNumber"][0]);
	if  err != nil {
		log.Fatal(err)
	}
	*/

	number = r.Form["phoneNumber"][0]

	lat, err := strconv.ParseFloat(r.Form["latitude"][0], 64);
	lon, err := strconv.ParseFloat(r.Form["longitude"][0], 64);

	if err != nil {
		log.Fatal(err)
	} else {
		location = Location{Latitude: lat, Longitude: lon}
	}
	
	tmp := Pickup{number, location, time.Now(), location, time.Now(), pending, time.Time{}, time.Time{}}

	pickups[number] = tmp

	if output, err := json.Marshal(pickups[number]); err == nil {
		fmt.Fprintf(w, string(output))
	} else {
		log.Fatal(err)
	}
}

func getPickupInfo(w http.ResponseWriter, r *http.Request) {
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	if !doKeysExist(r.Form, []string{"phoneNumber", "latitude", "longitude"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber", "latitude", "longitude"}) {
		log.Fatal("required http parameters not found for getPickupInfo")
	}

	var number string
	var location Location

	number = r.Form["phoneNumber"][0]

	if lat, err := strconv.ParseFloat(r.Form["latitude"][0], 64); err == nil {
		if lon, err := strconv.ParseFloat(r.Form["longitude"][0], 64); err == nil {
			location = Location{Latitude: lat, Longitude: lon}

			var tmp = pickups[number]
			tmp.LatestLocation = location
			tmp.LatestTime = time.Now()
			pickups[number] = tmp
		}
	}

	if output, err := json.Marshal(pickups[number]); err == nil {
		fmt.Fprintf(w, string(output[:]))
	} else {
		log.Fatal(err)
	}
}

func getPickupList(w http.ResponseWriter, r *http.Request) {
	//check passphrase in "phrase" parameter
	if !isPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if output, err := json.Marshal(pickups); err == nil {
		fmt.Fprintf(w, string(output[:]))
	} else {
		log.Fatal(err)
	}
}

func confirmPickup(w http.ResponseWriter, r *http.Request) {
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	//check passphrase in "phrase" parameter
	if !isPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if !doKeysExist(r.Form, []string{"phoneNumber"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber"}) {
		log.Fatal("required http parameters not found for confirmPickup")
	}

	var number string

	number = r.Form["phoneNumber"][0]

	var tmp = pickups[number]
	tmp.Status = confirmed
	tmp.ConfirmTime = time.Now()
	pickups[number] = tmp

	fmt.Fprintf(w, successResponse)
}

func completePickup(w http.ResponseWriter, r *http.Request) {
	//bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	//check passphrase in "phrase" parameter
	if !isPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if !doKeysExist(r.Form, []string{"phoneNumber"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber"}) {
		log.Fatal("required http parameters not found for completePickup")
	}

	var number string

	number = r.Form["phoneNumber"][0]

	var tmp = pickups[number]
	tmp.Status = completed
	tmp.CompleteTime = time.Now()
	pickups[number] = tmp
}

func updateVanLocation(w http.ResponseWriter, r *http.Request) {
	//bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	//check passphrase in "phrase" parameter
	if !isPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if !doKeysExist(r.Form, []string{"vanNumber", "latitude", "longitude"}) && areFieldsEmpty(r.Form ,[]string{"vanNumber", "latitude", "longitude"}) {
		log.Fatal("required http parameters not found for getPickupInfo")
	}

	var vanNumber int
	var location Location

	vanNumber, err := strconv.Atoi(r.Form["vanNumber"][0]);
	if  err != nil {
		log.Fatal(err)
	}

	//5 vans max, #1-5
	if vanNumber < 1 || vanNumber > 5 {
		if output, err := json.Marshal(Location{}); err == nil {
			fmt.Fprintf(w, string(output[:]))
		} else {
			log.Fatal(err)
		}
		return
	}

	lat, err := strconv.ParseFloat(r.Form["latitude"][0], 64);
	lon, err := strconv.ParseFloat(r.Form["longitude"][0], 64);
	if err != nil {
		log.Fatal(err)
	} else {
		location = Location{Latitude: lat, Longitude: lon}
	}

	for len(vanLocations) < vanNumber {
		vanLocations = append(vanLocations, Location{})
	}

	vanLocations[vanNumber - 1] = location

	//reply with van location on server
	if output, err := json.Marshal(vanLocations[vanNumber - 1]); err == nil {
		fmt.Fprintf(w, string(output[:]))
	} else {
		log.Fatal(err)
	}
}

func server(wg *sync.WaitGroup) {
	http.HandleFunc("/", uptimeHandler)

	//pickupee functions
	http.HandleFunc("/newPickup", newPickup)
	http.HandleFunc("/getPickupInfo", getPickupInfo)

	//driver functions
	http.HandleFunc("/getPickupList", getPickupList)
	http.HandleFunc("/confirmPickup", confirmPickup)
	http.HandleFunc("/completePickup", completePickup)
	http.HandleFunc("/updateVanLocation", updateVanLocation)

	
	//http.ListenAndServe(":8080", nil)
    
    err := http.ListenAndServe(":"+os.Getenv("PORT"), nil) 
    fmt.Println("Listening on " + os.Getenv("PORT"))
    if err != nil {
      log.Fatal(err)
    } 
}

func removeInactive(targetMap *map[string]Pickup, timeDifference time.Duration) {
	for k, v := range *targetMap {
		if time.Since(v.LatestTime) > timeDifference {
			//delete(*targetMap, k)
			v.Status = inactive
			(*targetMap)[k] = v
		}
	}
}

func main() {

	pickups = make(map[string]Pickup)
	vanLocations = make([]Location, 0)
	generateSuccessResponse(&successResponse)
	generateFailResponse(&failResponse)

	t := time.NewTicker(30 * time.Second)

	var wg sync.WaitGroup
	wg.Add(1)

	go server(&wg)

	fmt.Println("Finished setting up and ready.")

	for now := range t.C {
		now = now
		go removeInactive(&pickups, time.Duration(15) * time.Second)
	}

	wg.Wait()
}
