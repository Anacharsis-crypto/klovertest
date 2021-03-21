package temp_from_zip

import "testing"
import "os"
import "time"


func TestLatest(t *testing.T) {
    os.Setenv("OPENWEATHERMAP_API_KEY", "809f761fd91b3990cdc45262b01aa174")
    testStartTime := time.Now().Unix()
    t.Logf("TEST DRIVER: begin (t=%v)", testStartTime)

    // First make a basic call to verify the happy path woks
    got := Latest("90210")
    t.Logf("happy-path-check: zip 90210 (Beverly Hills California), got: %v", got)

    // Call 100x same symbol, time it, expect same value, report - expect pretty fast!
    //for some reason 22222 (arlington va) doesn't work? city not found..
    t.Logf("cache-speed-test: 100x for 06883 (Weston, CT)...")
    t2Start := time.Now().Unix()
    got = Latest("06883")
    for i:=1; i<=100; i++ {
        got2 := Latest("06883")
        if (got != got2) {
            t.Errorf("Initial state for 06883: %v, then got %v in 100 repeat calls, what's wrong?", got, got2)
        }

    }
    t2End := time.Now().Unix()
    t.Logf("cache-speed-test call 100x for 06883 produced identical: %v total time: %vs", got, (t2End-t2Start))

    t.Logf("rate-limit-test: 30 zip codes in a row, timed. expect to take ~62 seconds")
    // I verified these visually since apparently not all valid zip codes are supported
    codes := [30]string{ "90210", "06883", "06881", "06034", "06035", 
                            "48504", "06037", "48506", "06039", "06040",

                            "06041", "06042", "06043", "48507", "06045",
                            "48502", "98110", "98109", "98108", "06050",
                            "06051", "06052", "06053", "98107", "06055",
                            "98106", "06057", "06058", "06059", "06060"}

    t2Start = time.Now().Unix()
    for i:=0; i<30; i++ {
        c := codes[i]
        got = Latest(c)
        t.Logf("code: %v got: %v", c, got)
    }
    t2End = time.Now().Unix()
    t.Logf("rate-limit-test: total time: %vs", (t2End-t2Start))
}

