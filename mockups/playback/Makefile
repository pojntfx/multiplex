all: build

build:
	mkdir -p out
	cp video.webp out
	scss main.scss out/main.css
	blueprint-compiler compile --output out/window.ui window.blp
	esbuild --bundle --outfile=out/main.js '--external:gi://*' --platform=neutral main.js

run: build
	gjs -m out/main.js

clean:
	rm -rf out
