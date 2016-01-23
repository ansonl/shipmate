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
	_ "github.com/lib/pq"
  	"database/sql"
)

type Location struct {
	Latitude float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Heading float64 `json:"heading"`
	latestTime time.Time
}

//Status int constants
const pending int = 1
const confirmed int = 2
const completed int = 3
const inactive int = 0

const canceled int = 5 //only used when copying into pastPickups table

type Pickup struct {
	PhoneNumber string `json:"phoneNumber"`
	devicePhrase string
	InitialLocation Location `json:"initialLocation"`
	InitialTime time.Time `json:"initialTime"`
	LatestLocation Location `json:"latestLocation"`
	LatestTime time.Time `json:"latestTime"`
	ConfirmTime time.Time `json:"confirmTime"`
	CompleteTime time.Time `json:"completeTime"`
	Status int `json:"status"`
}

var pickups map[string]Pickup

var vanLocations []Location

var startTime = time.Now()

var successResponse string
var failResponse string
var wrongPasswordResponse string

var db *(sql.DB)

var serialChannel chan func();

func generateSuccessResponse(targetString *string) {
	tmp, err := json.Marshal(map[string]string{"status":"0"})
	*targetString = string(tmp)
	if  err != nil {
		fmt.Printf("Generating success response failed. %v", err)
	}
}

func generateFailResponse(targetString *string) {
	tmp, err := json.Marshal(map[string]string{"status":"-1"})
	*targetString = string(tmp)
	if  err != nil {
		fmt.Printf("Generating fail response failed. %v", err)
	}
}

func generateWrongPasswordResponse(targetString *string) {
	tmp, err := json.Marshal(map[string]string{"status":"-2"})
	*targetString = string(tmp)
	if  err != nil {
		fmt.Printf("Generating wrong password response failed. %v", err)
	}
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

func isDriverPhraseCorrect(targetDictionary url.Values) bool {
	if doKeysExist(targetDictionary, []string{"phrase"}) && !areFieldsEmpty(targetDictionary ,[]string{"phrase"}) {
		if checkMD5([]byte(targetDictionary["phrase"][0])) {
			return true
		} else {
			//fmt.Println("Wrong driver phrase \"" + targetDictionary["phrase"][0] + "\" received")
		}
	} else {
		fmt.Println("No phrase HTTP parameter received.")
	}
	return false
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	http.Redirect(w, r, "https://github.com/ansonl/shipmate", http.StatusFound)

	log.Println("About requested")
}

func uptimeHandler(w http.ResponseWriter, r *http.Request) {
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	diff := time.Since(startTime)

	fmt.Fprintf(w, "Uptime:\t%v\nPickups total:\t%v\nVans total:\t%v", diff.String(), len(pickups), len(vanLocations))

	log.Println("Uptime requested")
}

func newPickup(w http.ResponseWriter, r *http.Request) {
	log.Println("newPickup()")
    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	r.ParseForm()

	if !doKeysExist(r.Form, []string{"phoneNumber", "latitude", "longitude", "phrase"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber", "latitude", "longitude", "phrase"}) {
		log.Println("required http parameters not found for newPickup")
	}

	var number, devicePhrase string
	var location Location

	/*
	number, err := strconv.Atoi(r.Form["phoneNumber"][0]);
	if  err != nil {
		log.Println(err)
	}
	*/

	number = r.Form["phoneNumber"][0]
	devicePhrase = r.Form["phrase"][0]

	lat, err := strconv.ParseFloat(r.Form["latitude"][0], 64);
	lon, err := strconv.ParseFloat(r.Form["longitude"][0], 64);

	if err != nil {
		log.Println(err)
	} else {
		location = Location{Latitude: lat, Longitude: lon}
	}

	//if someone else if already using that number and devicePhrase does not match, maybe the user reinstalled the app
	//we want to allow the same device to continue using the phoneNumber if the app relaunched
	if pickups[number].Status != 0 && pickups[number].devicePhrase != "" && pickups[number].devicePhrase != devicePhrase{
		fmt.Fprintf(w, failResponse)
		return
	}

	tmp := Pickup{number, devicePhrase, location, time.Now(), location, time.Now(), time.Time{}, time.Time{}, pending}

	pickups[number] = tmp

	if output, err := json.Marshal(pickups[number]); err == nil {
		fmt.Fprintf(w, string(output))

		//INSERT pickup as new row into inprogress table
		serialChannel <- func() { databaseInsertPickupInCurrentTable(tmp) }
	} else {
		log.Println(err)
	}
}

func getPickupInfo(w http.ResponseWriter, r *http.Request) {
	/*
	//Disable logging for getPickupInfo for brevity
	log.Println("getPickupInfo()")
	*/

    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	if !doKeysExist(r.Form, []string{"phoneNumber", "latitude", "longitude", "phrase"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber", "latitude", "longitude", "phrase"}) {
		log.Println("required http parameters not found for getPickupInfo")
	}

	var number string
	var location Location

	number = r.Form["phoneNumber"][0]

	//if the pickup does not exist, return status 0, so that monitorStatus on iOS will show pickupInactive
	if _, exist := pickups[number]; !exist {
		fmt.Fprintf(w, successResponse)
		return
	}

	//check passphrase in "phrase" parameter
	if !isDriverPhraseCorrect(r.Form) {
		if r.Form["phrase"][0] != pickups[number].devicePhrase && pickups[number].devicePhrase != "" {
			fmt.Fprintf(w, wrongPasswordResponse)
			return
		}
	}

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

		//UPDATE pickup location in inprogress table
		serialChannel <- func() { databaseUpdatePickupLatestLocationInCurrentTable(number, location, time.Now())}
	} else {
		log.Println(err)
	}
}

func getVanLocations(w http.ResponseWriter, r *http.Request) {
	/*
	//Disabled logging of getVanLocations for brevity
	log.Println("getVanLocations()")
	*/

	//bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//reply with all van locations on server
	if output, err := json.Marshal(vanLocations); err == nil {
		fmt.Fprintf(w, string(output[:]))
	} else {
		log.Println(err)
	}
}

func cancelPickup(w http.ResponseWriter, r *http.Request) {
	log.Println("cancelPickup()")

    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	if !doKeysExist(r.Form, []string{"phoneNumber", "phrase"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber", "phrase"}) {
		log.Println("required http parameters not found for getPickupInfo")
	}

	var number string

	number = r.Form["phoneNumber"][0]

	//check passphrase in "phrase" parameter
	if !isDriverPhraseCorrect(r.Form) {
		if r.Form["phrase"][0] != pickups[number].devicePhrase && pickups[number].devicePhrase != ""{
			fmt.Fprintf(w, wrongPasswordResponse)
			return
		}
	}

	var tmp = pickups[number]

	//perform INSERT, DELETE in order
	//serialChannel <- func() { databaseUpdatePickupStatusInCurrentTable(number, canceled) } //canceled status (5) only shown in database, server app structs will never see it
	serialChannel <- func() { databaseInsertPickupInPastTable(tmp) }
	serialChannel <- func() { databaseDeletePickupInCurrentTable(number) }
	
	//do database before updating structin memory to preserve device phrase
	tmp.Status = inactive
	tmp.LatestTime = time.Now()
	tmp.devicePhrase = ""
	pickups[number] = tmp

	fmt.Fprintf(w, successResponse);

	
}

func getPickupList(w http.ResponseWriter, r *http.Request) {
	log.Println("getPickupList()")

	//bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	//check passphrase in "phrase" parameter
	if !isDriverPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if output, err := json.Marshal(pickups); err == nil {
		fmt.Fprintf(w, string(output[:]))
	} else {
		log.Println(err)
	}
}

func confirmPickup(w http.ResponseWriter, r *http.Request) {
    log.Println("confirmPickup()")

    //bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	//check passphrase in "phrase" parameter
	if !isDriverPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if !doKeysExist(r.Form, []string{"phoneNumber"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber"}) {
		log.Println("required http parameters not found for confirmPickup")
	}

	var number string

	number = r.Form["phoneNumber"][0]

	var tmp = pickups[number]
	tmp.Status = confirmed
	tmp.ConfirmTime = time.Now()
	pickups[number] = tmp

	fmt.Fprintf(w, successResponse)

	serialChannel <- func() { databaseUpdatePickupStatusInCurrentTable(number, confirmed)}
}

func completePickup(w http.ResponseWriter, r *http.Request) {
	log.Println("completePickup()")

	//bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	//check passphrase in "phrase" parameter
	if !isDriverPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if !doKeysExist(r.Form, []string{"phoneNumber"}) && areFieldsEmpty(r.Form ,[]string{"phoneNumber"}) {
		log.Println("required http parameters not found for completePickup")
	}

	var number string

	number = r.Form["phoneNumber"][0]

	var tmp = pickups[number]
	tmp.Status = completed
	tmp.CompleteTime = time.Now()
	pickups[number] = tmp

	fmt.Fprintf(w, successResponse)

	//perform UPDATE, INSERT, DELETE in order
	serialChannel <- func() { databaseUpdatePickupStatusInCurrentTable(number, completed) }
	serialChannel <- func() { databaseInsertPickupInPastTable(tmp) }
	serialChannel <- func() { databaseDeletePickupInCurrentTable(number) }
}

//INSERT new pickup row in inprogress table
func databaseInsertPickupInCurrentTable(targetPickup Pickup) {
	if db != nil {
		if err := db.Ping(); err == nil {
			var result sql.Result
			if result, err = db.Exec("INSERT INTO inprogress (PhoneNumber, DeviceId, InitialLatitude, InitialLongitude, InitialTime, LatestLatitude, LatestLongitude, LatestTime, ConfirmTime, CompleteTime, Status) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);", targetPickup.PhoneNumber, targetPickup.devicePhrase, targetPickup.InitialLocation.Latitude, targetPickup.InitialLocation.Longitude, targetPickup.InitialTime, targetPickup.LatestLocation.Latitude, targetPickup.LatestLocation.Longitude, targetPickup.LatestTime, targetPickup.ConfirmTime, targetPickup.CompleteTime, targetPickup.Status); err != nil {
				log.Println(err)
			} else {
				rowsAffected, _ := result.RowsAffected()
				fmt.Printf("INSERT %v rows affected for databaseInsertPickupInCurrentTable()\n", rowsAffected)
			}
		} else {
			fmt.Println("DB ping failed.")
		}
	} else {
		log.Println("DB handle is nil")
	}
}

//UPDATE pickup status in inprogress table
func databaseUpdatePickupStatusInCurrentTable(targetPhoneNumber string, targetStatus int) {
	if db != nil {
		if err := db.Ping(); err == nil {
			var result sql.Result
			if result, err = db.Exec("UPDATE inprogress SET Status = $1 WHERE PhoneNumber = $2;", targetStatus, targetPhoneNumber); err != nil {
				log.Println(err)
			} else {
				rowsAffected, _ := result.RowsAffected()
				fmt.Printf("UPDATE %v rows affected for databaseUpdatePickupStatusInCurrentTable()\n", rowsAffected)
			}
		} else {
			fmt.Println("DB ping failed.")
		}
	} else {
		log.Println("DB handle is nil")
	}
}

//UPDATE pickup latestLocation in inprogress table
func databaseUpdatePickupLatestLocationInCurrentTable(targetPhoneNumber string, targetLatestLocation Location, targetTime time.Time) {
	if db != nil {
		if err := db.Ping(); err == nil {
			var result sql.Result
			if result, err = db.Exec("UPDATE inprogress SET LatestLatitude = $1, LatestLongitude = $2, LatestTime = $3 WHERE PhoneNumber = $4;", targetLatestLocation.Latitude, targetLatestLocation.Longitude, targetTime, targetPhoneNumber); err != nil {
				log.Println(err)
			} else {
				rowsAffected, _ := result.RowsAffected()
				fmt.Printf("UPDATE %v rows affected for databaseUpdatePickupLatestLocationInCurrentTable()\n", rowsAffected)
			}
		} else {
			fmt.Println("DB ping failed.")
		}
	} else {
		log.Println("DB handle is nil")
	}
}

//Copy over to pastpickups table and call function to delete from inprogress table
func databaseInsertPickupInPastTable(targetPickup Pickup) {
	if db != nil {
		if err := db.Ping(); err == nil {
			var result sql.Result
			if result, err = db.Exec("INSERT INTO pastpickups (PhoneNumber, DeviceId, InitialLatitude, InitialLongitude, InitialTime, LatestLatitude, LatestLongitude, LatestTime, ConfirmTime, CompleteTime, Status) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);", targetPickup.PhoneNumber, targetPickup.devicePhrase, targetPickup.InitialLocation.Latitude, targetPickup.InitialLocation.Longitude, targetPickup.InitialTime, targetPickup.LatestLocation.Latitude, targetPickup.LatestLocation.Longitude, targetPickup.LatestTime, targetPickup.ConfirmTime, targetPickup.CompleteTime, targetPickup.Status); err != nil {
				log.Println(err)
			} else {
				rowsAffected, _ := result.RowsAffected()
				fmt.Printf("INSERT %v rows affected for databaseInsertPickupInPastTable()\n", rowsAffected)
			}
		} else {
			fmt.Println("DB ping failed.")
		}
	} else {
		log.Println("DB handle is nil")
	}
}

//DELETE pickup from inprogress table
func databaseDeletePickupInCurrentTable(targetPhoneNumber string) {
	if db != nil {
		if err := db.Ping(); err == nil {
			var result sql.Result
			if result, err = db.Exec("DELETE FROM inprogress WHERE PhoneNumber = $1;", targetPhoneNumber); err != nil {
				log.Println(err)
			} else {
				rowsAffected, _ := result.RowsAffected()
				fmt.Printf("DELETE %v rows affected for databaseDeletePickupInCurrentTable()\n", rowsAffected)
			}
		} else {
			fmt.Println("DB ping failed.")
		}
	} else {
		log.Println("DB handle is nil")
	}
}

func updateVanLocation(w http.ResponseWriter, r *http.Request) {
	log.Println("updateVanLocation()")

	//bypass same origin policy
	w.Header().Set("Access-Control-Allow-Origin", "*")

	//parse http parameters
	r.ParseForm()

	//check passphrase in "phrase" parameter
	if !isDriverPhraseCorrect(r.Form) {
		fmt.Fprintf(w, failResponse)
		return
	}

	if !doKeysExist(r.Form, []string{"vanNumber", "latitude", "longitude"}) && areFieldsEmpty(r.Form ,[]string{"vanNumber", "latitude", "longitude"}) {
		log.Println("required http parameters not found for updateVanLocation")
	}

	var vanNumber int
	var location Location

	vanNumber, err := strconv.Atoi(r.Form["vanNumber"][0]);
	if  err != nil {
		log.Println(err)
	}

	//5 vans max, #1-5
	if vanNumber < 1 || vanNumber > 5 {
		if output, err := json.Marshal(Location{}); err == nil {
			fmt.Fprintf(w, string(output[:]))
		} else {
			log.Println(err)
		}
		return
	}

	lat, err := strconv.ParseFloat(r.Form["latitude"][0], 64);
	lon, err := strconv.ParseFloat(r.Form["longitude"][0], 64);
	if err != nil {
		log.Println(err)
	} else {
		location = Location{Latitude: lat, Longitude: lon}
	}

	if doKeysExist(r.Form, []string{"heading"}) && !areFieldsEmpty(r.Form ,[]string{"heading"}) {
		heading, err := strconv.ParseFloat(r.Form["heading"][0], 64);
		if err != nil {
			log.Println(err)
		} else {
			location.Heading = heading
		}
	} else {
		location.Heading = -1
	}

	for len(vanLocations) < vanNumber {
		vanLocations = append(vanLocations, Location{})
	}

	vanLocations[vanNumber - 1] = location

	vanLocations[vanNumber - 1].latestTime = time.Now()

	//reply with van location on server
	if output, err := json.Marshal(vanLocations[vanNumber - 1]); err == nil {
		fmt.Fprintf(w, string(output[:]))
	} else {
		log.Println(err)
	}
}

func server(wg *sync.WaitGroup) {
	//general functions
	http.HandleFunc("/", aboutHandler)
	http.HandleFunc("/uptime", uptimeHandler)

	//pickupee functions
	http.HandleFunc("/newPickup", newPickup)
	http.HandleFunc("/getPickupInfo", getPickupInfo)
	http.HandleFunc("/getVanLocations", getVanLocations)

	//shared functions
	http.HandleFunc("/cancelPickup", cancelPickup)

	//driver functions
	http.HandleFunc("/getPickupList", getPickupList)
	http.HandleFunc("/confirmPickup", confirmPickup)
	http.HandleFunc("/completePickup", completePickup)
	http.HandleFunc("/updateVanLocation", updateVanLocation)

	//bind to $PORT environment variable
  err := http.ListenAndServe(":"+os.Getenv("PORT"), nil) 
  fmt.Println("Listening on " + os.Getenv("PORT"))
  if err != nil {
    log.Println(err)
  } 

  wg.Done()
}

//anything that is not inactive is set to inactive
func removeInactivePickups(targetMap *map[string]Pickup, timeDifference time.Duration) {
	for k, v := range *targetMap {
		if v.Status != inactive && time.Since(v.LatestTime) > timeDifference { //only check active pickups
			//delete(*targetMap, k) do not delete, because we want to preserve the pickup records
			v.Status = inactive
			v.devicePhrase = ""
			(*targetMap)[k] = v

			//perform UPDATE, INSERT, DELETE in order
			serialChannel <- func() { databaseUpdatePickupStatusInCurrentTable(v.PhoneNumber, inactive) }
			serialChannel <- func() { databaseInsertPickupInPastTable(v) }
			serialChannel <- func() { databaseDeletePickupInCurrentTable(v.PhoneNumber) }
				}
	}
}

func removeInactiveVanLocations(targetArray []Location, timeDifference time.Duration) {
	var numberOfEmptyLocations int

	for i := 0; i < len(targetArray); i++ {
		
		if (targetArray[i].latestTime != time.Time{} && time.Since((targetArray)[i].latestTime) > timeDifference) {
			fmt.Println(time.Since((targetArray)[i].latestTime))
			fmt.Println(timeDifference)

			targetArray[i].Latitude = 0
			targetArray[i].Longitude = 0
			targetArray[i].latestTime = time.Time{}
			numberOfEmptyLocations++

		} else if (targetArray[i].latestTime == time.Time{}) {
			numberOfEmptyLocations++
		}
	}

	//if there are 'len' empty locations in the array, no vans are around so realloc array to size 0
	if numberOfEmptyLocations == len(targetArray) {
		vanLocations = make([]Location, 0)
	}
}

func checkForInactive(wg *sync.WaitGroup) {
	t := time.NewTicker(time.Duration(10) * time.Second)
	for now := range t.C {
		now = now
		go removeInactivePickups(&pickups, time.Duration(10) * time.Minute)
		go removeInactiveVanLocations(vanLocations, time.Duration(1) * time.Minute)
	}
	wg.Done()
}

func setupCurrentDatabase() {
	var err error //define err because mixing it with the global db var and := operator creates local scoped db
	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Println(err)
	}

	var tableExist bool
	err = db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND    table_name = 'inprogress');").Scan(&tableExist)
	if err != nil {
		log.Println(err)
	}
	if tableExist { //load in inprogress pickups from database
		go func() {
			rows, err := db.Query("SELECT * from inprogress;")
			if err != nil {
				log.Println(err)
			}
			var countOfRows = 0
			for rows.Next() {
				var pickupId int
				var tmpPickup Pickup

				if err := rows.Scan(&pickupId, &tmpPickup.PhoneNumber, &tmpPickup.devicePhrase, &tmpPickup.InitialLocation.Latitude, &tmpPickup.InitialLocation.Longitude, &tmpPickup.InitialTime, &tmpPickup.LatestLocation.Latitude, &tmpPickup.LatestLocation.Longitude, &tmpPickup.LatestTime, &tmpPickup.ConfirmTime, &tmpPickup.CompleteTime, &tmpPickup.Status); err != nil {
					log.Println(err)
				}

				/*
				//Currently no way to handle a column that can be time.Time or NULL

				var tmpConfirmTime sql.NullString
				var tmpCompleteTime sql.NullString

				layout := "2016-01-19 22:25:13.047371"
				if tmpConfirmTime.Valid {
					timeStamp, err := time.Parse(layout, tmpConfirmTime.String)
					if err != nil {
						tmpPickup.ConfirmTime = timeStamp
					} else {
						tmpPickup.ConfirmTime = time.Time{}
					}
				} else {
					tmpPickup.ConfirmTime = time.Time{}
				}

				if tmpCompleteTime.Valid {
					timeStamp, err := time.Parse(layout, tmpCompleteTime.String)
					if err != nil {
						tmpPickup.CompleteTime = timeStamp
					} else {
						tmpPickup.CompleteTime = time.Time{}
					}
				} else {
					tmpPickup.CompleteTime = time.Time{}
				}
				*/
				fmt.Printf("Loaded existing pickup for %v\n", tmpPickup.PhoneNumber)
				pickups[tmpPickup.PhoneNumber] = tmpPickup
				countOfRows++
			}
			rows.Close()
			log.Printf("Finished loading %v pickups.\n", countOfRows)
		}()

	} else { //create new inprogress database
		if _, err = db.Exec("CREATE TABLE inprogress (PickupId SERIAL,PhoneNumber CHAR(10) NOT NULL,DeviceId VARCHAR(36) NOT NULL,InitialLatitude REAL NOT NULL,InitialLongitude REAL NOT NULL,InitialTime TIMESTAMP NOT NULL,LatestLatitude REAL NOT NULL,LatestLongitude REAL NOT NULL,LatestTime TIMESTAMP NOT NULL,ConfirmTime TIMESTAMP NOT NULL,CompleteTime TIMESTAMP NOT NULL,Status INT NOT NULL,CONSTRAINT PK_PickupIdInProgress PRIMARY KEY (PickupId),CONSTRAINT Check_PhoneNumber CHECK (CHAR_LENGTH(PhoneNumber) = 10));"); err != nil {
			log.Println(err)
		} else {
			log.Println("CREATE TABLE inprogress executed\n")
		}
	}
}

func setupPastDatabase() {
	var err error //define err because mixing it with the global db var and := operator creates local scoped db
	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Println(err)
	}

	var tableExist bool
	err = db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'pastpickups');").Scan(&tableExist)
	if err != nil {
		log.Println(err)
	}
	if !tableExist {
		if _, err = db.Exec("CREATE TABLE pastpickups (PickupId SERIAL,PhoneNumber CHAR(10) NOT NULL,DeviceId VARCHAR(36) NOT NULL,InitialLatitude REAL NOT NULL,InitialLongitude REAL NOT NULL,InitialTime TIMESTAMP NOT NULL,LatestLatitude REAL NOT NULL,LatestLongitude REAL NOT NULL,LatestTime TIMESTAMP NOT NULL,ConfirmTime TIMESTAMP NOT NULL,CompleteTime TIMESTAMP NOT NULL,Status INT NOT NULL,CONSTRAINT Check_PhoneNumber CHECK (CHAR_LENGTH(PhoneNumber) = 10));"); err != nil {
			log.Println(err)
		} else {
			log.Println("CREATE TABLE pastpickups executed\n")
		}
	}
}

func main() {

	pickups = make(map[string]Pickup)
	vanLocations = make([]Location, 0)
	generateSuccessResponse(&successResponse)
	generateFailResponse(&failResponse)
	generateWrongPasswordResponse(&wrongPasswordResponse)

	//create channel of function type
	serialChannel = make(chan func())
	//spawn go routine to continuously read and run functions in the channel
	go func() {
		for true {
			tmp := <-serialChannel
			tmp()
		}
	}()
	/*
	//test the serialChannel
	serialChannel <- func() { log.Println("i=1")}
	serialChannel <- func() { log.Println("i=2")}
	serialChannel <- func() { log.Println("i=3")}
	serialChannel <- func() { log.Println("i=4")}
	*/

	var wg sync.WaitGroup
	wg.Add(2)

	go server(&wg)
	//go checkForInactive(&wg)
	
	/*
	result, err = db.Exec("INSERT INTO inprogress (PhoneNumber, DeviceId, InitialLatitude, InitialLongitude, InitialTime, LatestLatitude, LatestLongitude, LatestTime, ConfirmTime, CompleteTime, Status) VALUES ('5103868680', '68753A44-4D6F-1226-9C60-0050E4C00067', 38.9844, 76.4889, '2002-10-02T10:00:00-05:00', 38.9844, 76.4889, '2002-10-02T10:00:00-05:00', DEFAULT, DEFAULT, 0); ")
	fmt.Println(result)
	*/

	setupCurrentDatabase()
	setupPastDatabase()

	fmt.Println("Finished setting up and ready. Loading existing pickups stored on database into memory may be happening in background.")

	wg.Wait()

	err := db.Close()
	if err != nil {
		log.Println(err)
	}
}
