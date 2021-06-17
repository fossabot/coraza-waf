---
title: Implementing Coraza WAF
keywords: implementation, apache, coraza, waf, opensource
last_updated: June 14, 2021
sidebar: mydoc_sidebar
permalink: implementing.html
folder: mydoc
---

## Serving HTTP workflow

1. Indicate request line and request headers
2. Evaluate phase 1
3. Indicate request body
4. Evaluate phase 2
5. Send request and accept response
6. Indicate response headers
7. Evaluate phase 3
8. Indicate response body
9. Evaluate phase 4
10. Evaluate phase 5

## Important considerations

* Phase 5 must always be evaluated
* If you skip a phase, the next evaluated phase will also evaluate the previous phase
* Interruptions depends on the implementation

## Implementing a base HTTP server

{% highlight golang %}
package main

import (
    "fmt"
    "log"
    "net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "My golang web server")
}

func main() {
    http.HandleFunc("/", handler)
    log.Fatal(http.ListenAndServe(":8080", nil))
}
{% endhighlight %}

## Implementing a WAF instance

{% highlight golang %}
package main
import(
	"github.com/jptosso/coraza-waf/pkg/engine"
	"github.com/jptosso/coraza-waf/pkg/seclang"
)

// *engine.Waf is a routine friendly implementation but requires initialization
const waf *engine.Waf
func initWaf(conf string) error{
	waf.Init()
	parser := seclang.NewParser(waf)
	retturn parser.FromFile(conf)
}
{% endhighlight %}

## Processing request and response objects

{% highlight golang %}
func handler(w http.ResponseWriter, r *http.Request) {
	// a new transaction is required for each request
	tx := waf.NewTransaction()
	tx.ParseRequestObject(r)
	tx.ExecutePhase(2) // it will evaluate phase 1 and 2
	if tx.Interrupted() {
		w.WriteHeader(403)
		fmt.Fprintf(w, "Request was interrupted")
		tx.ExecutePhase(5)
		return
	}
	tx.ParseResponseHeaders(w.Header())
	tx.ExecutePhase(3)
	// We can only implement phase 3 interruptions if headers weren't already sent
	if tx.Interrupted() {
		w.WriteHeader(403)
		fmt.Fprintf(w, "Response was interrupted")
		tx.ExecutePhase(5)
		return
	}
	fmt.Fprintf(w, "My golang web server")
	// We may not implement phase 4 in this example because we cannot access the response object
	// To see a real phase 4 implementation check the reverse proxy example
	tx.ExecutePhase(5)
}

func main() {
	if err := initWaf("/some/path/to/config.conf"); err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
{% endhighlight %}

If content injection is enabled Coraza may directtly edit your response body and response headers.

## Implementing Interruptions

In order to implement interruptions, each use case must be implemented, these are:

* **Deny:** Interrupt the request and respond with a status code ``i.Status``
* **Drop (Cannot be implemented in this case):** Used to prevent DOS, if possible, RST the http request and stop processing it
* **Redirect:** Interrupt the request, send status 307, and redirect the user to ``i.Data``
* **Proxy:** Not implemented yet

{% highlight golang %}

func processInterruption(w http.ResponseWriter, tx *engine.Transaction) {
	in := tx.GetInterruption()
	switch in.Action {
		case "drop":
			// same as deny for this case
			w.WriteHeader(in.Status)
			fmt.Fprintf(w, "Transaction disrupted")
			break
		case "deny":
			w.WriteHeader(in.Status)
			fmt.Fprintf(w, "Transaction disrupted")
			break
		case "redirect":
			w.WriteHeader(307)
			w.Header().Set("Location", in.Data)
			fmt.Fprintf(w, "Redirected")
			break
	}
}
{% endhighlight %}

Now we replace our phase ``if tx.Interrupted()...`` with:
{% highlight golang %}
if tx.Interrupted() {
	processInterruption(w, tx)
	return
}
{% endhighlight %}	

## Final considerations

You might download this project from (https://github.com/jptosso/coraza-waf-sample-serve)[#]
