# Klover Code test submission from Jon North [![GoDoc](
### https://github.com/Anacharsis-crypto/klovertest

### Installation

Once you have [installed Go][golang-install], run this command
to install the `klovertest` package:

    go get github.com/Anacharsis-crypto/klovertest
    
### Documentation

#### Problem Statement
Write a library in Golang that exposes a method which takes a zip code as an argument and
returns the current temperature, humidity, and wind speed for that zip code pulled from the open
weather api ​https://openweathermap.org/current​. The library should cache requests on a per zip
code per hour basis and should additionally return the age of data for that zip code. Please use
809f761fd91b3990cdc45262b01aa174​ for the api key for openweathermap.

Deliverable
A link to a github repo or zip file containing your go code and instructions for how to import and
call your library.
Considerations
- Thread-safety
- Resource utilization
- Rate-limiting
- Modularization
- Cache management

#### Introduction / Running the driver
Library interface:
func Latest(zip string) AreaWeatherData

return value: struct of form:
type AreaWeatherData struct {
    temp string
    humidity string
    windSpeed string
    dataAgeSeconds string
    error string    
}
see code for commentary on members (top of file)

====

Run recommendation:
go test -v

Some logs were left in to make clear what's happening - it will produce a fair amount of text, but the important stuff is at the top and bottom:

temp_from_zip_test.go:51: call 100x for 06883 produced identical: {284.339996 37 1.110000 0 } total time: 0s

temp_from_zip_test.go:53: rate-limit-test: 30 zip codes in a row, timed. expect to take ~62 seconds
temp_from_zip_test.go:70: rate-limit-test: total time: 61s

--- PASS: TestLatest (60.49s)
PASS
ok      klovertest.com/zft      77.517s


I also tested bogus zip codes (since many valid zip codes aren't supported). The library returns all zeroes in that case, but is not currently polished/verbose in its error return.


#### Commentary
Thread-safety

Because of the mutex on the ratelimiting code and otherwise no interaction between threads, there should be no concerns here. The ratelimiting code is pretty fast, I do not expect meaningful contention at loads relevant to an implementation like this. A larger scale system would probably pull all relevant zip codes in batch to allow independent scaling of call volume vs poll volume

Resource utilization

Minimal.  By using a dict and keeping exactly one record per zip, it becomes a non issue. There are not enough zips (or postal codes) in the entire world to blow any conceivable modern memory.  
CPU use here is also minimal - hashmap lookup is O(1) so as the data store grows, runtimes will be roughly the same in any reasonable deployment.

Rate-limiting

See commentary in the code. I built a system you can easily adjust to any pricing tier (though it would be more sane to batch pull all zips if your SLA is going to be low milliseconds - decouple polling their site and serving requests to improve reliability and consistency)

Modularization

not being a Golang expert maybe this means something else, but there is so little code here I don't think it's meaningful to talk about.

Cache management

with a single cache entry for each zip and the ratelimiting data structure limited to the number per minute, there is not much management to do, and that is by design.


Overall this was fun and gave me a chance to learn more about golang, thanks! I'd appreciate feedback (idiomatic diversions, generally if you felt like there was a problem in my submission, etc).
