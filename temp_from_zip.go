package temp_from_zip

import "os"
import "fmt"
import "strconv"
import "time"
import "sync"
import "container/list"
import "net/http"
import "io/ioutil"
import "encoding/json"


type AreaWeatherData struct {
    // strings hold whatever comes over the wire - this code stays agnostic
    temp string
    humidity string
    windSpeed string

    // as measured from the return of the API call
    // (hence 0 in the case of a direct pull)
    dataAgeSeconds string
    // convention: if error is not empty, the values above are undefined.
    // valid returns will include an empty error string.
    error string    
}


type CacheEntry struct {
    temp string
    humidity string
    windSpeed string
    timestamp int64 // epoch seconds
}


// capitals due to golangs exceedingly arbitrary rule around exports needing that
type OWMMain struct {
    Temp float32
    Humidity int // visual inspection - never saw fractional percent
}


type OWMWind struct {
    Speed float32
}


type OWMResponse struct {
    Main OWMMain
    Wind OWMWind
}


// primitive ratelimiting scheme
type RateLimiter struct {
    mu sync.Mutex
    pullTimes list.List
}


var CACHE map[string]CacheEntry = make(map[string]CacheEntry)
// bounce the server to update, uses default 809f761fd91b3990cdc45262b01aa174​ if not present
var API_KEY string = ""
var PULL_URL string = "https://api.openweathermap.org/data/2.5/weather?zip=%s,us&appid=%s"


// obviously I could put these in env vars too, the above was to make a point.
// avoid "but your data is 31 minutes old, I round up so that's
// one hour and you said it was 0 hours old" complaints
// by pulling anytime data is 29 minutes old or older,
// we can always try to report "this hour's" data 
// (barring errors)
var CACHE_MAX_VALID_SECONDS int64 = 60 * 29

// openweathermap rate limits based on pricing tier
// For the moment let's assume we are using free tier.
// their policy: https://openweathermap.org/price
// 60 calls/min, 1M/month
// to keep it simple and provide a reasonable SLA,
// let's further limit so we don't have to keep track.
// 60 * 24 * 31 = 44,640 minutes in a month max (2.6m minutes)
// if we limit ourselves to 20 calls/min, we end up at 892.8k
// calls in a month, max, so no chance of blowing the per month limit.
// so we'll do that.
// nb this only affects outgoing calls, no limit is placed
// on the number of calls to this library's entry point.
// limiting at that level is up to the calling code.
var RATE_LIMIT_MAX_PULLS_MINUTE int64 = 20
var RATE_LIMIT_TRACKING_MAX_SECS int64 = 61 


// see func _WaitIfNecessary for usage
var RATE_LIMITER RateLimiter


// suggest versioning the api at a higher level (website level)
// other options are to embed versioning in the API name (LatestV1)
// or in the parameters/return payload.  I prefer versioning at 
// perimeter to keep low level code free of those considerations.
func Latest(zip string) AreaWeatherData {
    // Update environmment information (hot config update)
    if (API_KEY == "") {
        API_KEY = os.Getenv("OPENWEATHERMAP_API_KEY")
    }
    // log at debug/trace in prod
    //fmt.Printf("enter Latest(%s), api key: %s\n", zip, API_KEY)

    // Validate input
    validatedZip, valid := _Validate(zip)
    if(!valid) {
        ret := AreaWeatherData{}
        ret.error = fmt.Sprintf("Invalid zip(%s), accepted values are 00000 to 99999", zip)
        fmt.Printf("exit Latest(%s) invalid entry\n", zip)
        return ret
    }
    //fmt.Printf("zip %s accepted\n", validatedZip)

    // Look for good-enough cache entries
    cachedData, ok := CACHE[validatedZip]
    errorString := ""

    if (ok) {
        cachedDataAge := _CalcAge(cachedData)
        //fmt.Printf("cache hit, data age: %v vs max: %v\n", cachedDataAge, CACHE_MAX_VALID_SECONDS)
        if(cachedDataAge > CACHE_MAX_VALID_SECONDS) {
            ok = false
        }
    } else {
        cachedData = CacheEntry{}
        errorString = fmt.Sprintf("No data for zip %s\n", validatedZip)
    }

    // Remote pull data if no good cache entry
    if(!ok) {
        // every time we're going to pull, track the fact that we did so,
        // and wait long enough to be certain we're respecting ratelimits
        _WaitIfNecessary()
        updatedData, ok := _Pull(validatedZip)
        if (!ok) {
            // _Pull has already logged failure
            // return what we've got, could final log here.
            // valid return since we've got no SLA
            // and we do have a cached result (no error log in ret)
            ret := _PopulateReturn(cachedData, "")
            fmt.Printf("_Pull failed, returning cached entry: %v\n", ret)
            return ret
        }
        fmt.Printf("pull succeeded (outer), new cache value: %v\n", updatedData)
        errorString = ""
        // update cache and our local variable.
        cachedData = updatedData
        CACHE[validatedZip] = updatedData
    } else {
    }


    // however we got here, provide what we have.
    ret := _PopulateReturn(cachedData, errorString)
    //fmt.Printf("main return case, returning: %v\n", ret)
    return ret
}


// given a cache entry, compute data age
func _CalcAge(entry CacheEntry) int64 {
    secs := time.Now().Unix()
    return entry.timestamp - secs
}


func _Validate(zip string) (string, bool) {
    // only accept 5 digits. could be fancy and try to determine if a valid
    // zip, I'd keep a list of zipcodes around, initialized dynamically at
    // init time, rather than doing a hot lookup. the failure mode there is 
    // a new zipcode is published on a Friday and the server stays up over the
    // weekend.. i dont really see the problem with some bogus lookups
    asInt, err := strconv.Atoi(zip)
    if err == nil {
        return zip, (asInt >= 0) && (asInt <= 99999)
    }
    return "ERROR", false
}


func _Pull(validatedZip string) (CacheEntry, bool) {
    ret := CacheEntry{}
    // retry would be desirable in prod - not in a code test like this
    // sample api call (if you want to look at JSON for updating the code):
    // api.openweathermap.org/data/2.5/weather?zip=90210,us&appid=809f761fd91b3990cdc45262b01aa174

    // no real fault tolerance here, idk golang well enough
    // to do it idomatically. a bad url (eg nohttps://) panics.
    reqString := fmt.Sprintf(PULL_URL, validatedZip, API_KEY)
    fmt.Printf("https requesting: %s",  reqString)
    resp, err := http.Get(reqString)
    respBody, err2 := ioutil.ReadAll(resp.Body)

    if (err != nil || err2 != nil) {
        fmt.Printf("ERROR: response: \n%s\n\n err1: %s err2: %s\n", respBody, err, err2)
        return ret, false 
    }

    // The receiving structs are set up to pull the fields we need.
    var result OWMResponse
    jsonErr := json.Unmarshal([]byte(respBody), &result)
    if jsonErr != nil {
        fmt.Printf("pull failed to parse response: %v\n", result)
        return ret, false
    }
    ret.temp = fmt.Sprintf("%f", result.Main.Temp)
    ret.humidity = strconv.Itoa(result.Main.Humidity)
    ret.windSpeed = fmt.Sprintf("%f", result.Wind.Speed)
    nowEpochSecs := time.Now().Unix()
    ret.timestamp = nowEpochSecs

    fmt.Printf("pull succeeded, new data for %s: %v\n", validatedZip, ret)
    return ret, true 
}


// Type conversion: remove private data and add error verbosity
func _PopulateReturn(entry CacheEntry, errorString string) AreaWeatherData {
    ret := AreaWeatherData{temp:entry.temp, humidity:entry.humidity, windSpeed:entry.windSpeed}
    ret.dataAgeSeconds = strconv.Itoa(int(entry.timestamp - time.Now().Unix()))
    ret.error = errorString
    return ret
}


// see func _WaitIfNecessary for implementation
func _WaitIfNecessary() {
    // plan: lock the mutex,figure out if we've called too much lately,
    // if so hold in place till we have time, and then return.
    // This will have the effect of causing other threads to not
    // even be able to obtain the mutex till we're done,
    // which is the desired behavior (the first waiting thread
    // will get our clean slate +1 call, etc)
    RATE_LIMITER.mu.Lock()
    defer RATE_LIMITER.mu.Unlock()

    nowEpochSecs := time.Now().Unix()

    L := RATE_LIMITER.pullTimes.Len()
    fmt.Printf("_WaitIfNecessary stats: recent pull count: %v\n", L)
    

    // cleanup rate limit tracking data structure
    for RATE_LIMITER.pullTimes.Len() > 0 {
        oldest := RATE_LIMITER.pullTimes.Front().Value.(int64)

        delta := nowEpochSecs - oldest
        if(delta > RATE_LIMIT_TRACKING_MAX_SECS) {
            fmt.Printf("_WaitIfNecessary: delta: %v removing oldest pull: %v\n", delta, oldest)

            RATE_LIMITER.pullTimes.Remove(RATE_LIMITER.pullTimes.Front())
        } else {
            // there were no pulls older than our limit, done cleaning up.
            break
        }
    }

    // if we're under the rate limit, just go for it.
    recentPullCount := int64(RATE_LIMITER.pullTimes.Len())
    if (recentPullCount < RATE_LIMIT_MAX_PULLS_MINUTE) {
        RATE_LIMITER.pullTimes.PushBack(nowEpochSecs)
        L = RATE_LIMITER.pullTimes.Len()
        fmt.Printf("_WaitIfNecessary early return, not enough calls to worry about.  list length: %v\n", L) 
        return
    }

    // because of the mutex and the algo,
    // we know L.Len() == 20.
    // so all we have to do to be compliant is wait 
    // till the oldest call is outside the window
    oldestSecs := RATE_LIMITER.pullTimes.Front().Value.(int64)    
    // eg 61 - 500021 - 500005 -> wait 35s
    waitSecs := RATE_LIMIT_TRACKING_MAX_SECS - (nowEpochSecs - oldestSecs)
    waitDuration := time.Duration(waitSecs * 1e9) //nanosecond conversion, sigh
    fmt.Printf("_WaitIfNecessary sleeping to avoid ratelimit: %vs\n", waitSecs) 

    time.Sleep(waitDuration)
    //next iteration will clean up oldest call
}
