# Vintangle Settings Tool

First, compile the schema: `glib-compile-schemas cmd/vintangle-settings/`

Now, export the lookup path: `export GSETTINGS_SCHEMA_DIR=${PWD}/cmd/vintangle-settings/`

And run the tool: `go run ./cmd/vintangle-settings`
