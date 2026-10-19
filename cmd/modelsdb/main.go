// Command modelsdb is the ModelsDB server and its command line. All behaviour
// lives in internal/app; this file only starts it.
package main

import "modelsdb/internal/app"

func main() {
	app.Run()
}
