# Vintangle

![Logo](./docs/logo-readme.png)

Synchronized torrent streaming for distributed watch parties.

[![hydrun CI](https://github.com/pojntfx/vintangle/actions/workflows/hydrun.yaml/badge.svg)](https://github.com/pojntfx/vintangle/actions/workflows/hydrun.yaml)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.18-61CFDD.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/pojntfx/vintangle.svg)](https://pkg.go.dev/github.com/pojntfx/vintangle)
[![Matrix](https://img.shields.io/matrix/vintangle:matrix.org)](https://matrix.to/#/#vintangle:matrix.org?via=matrix.org)
[![Binary Downloads](https://img.shields.io/github/downloads/pojntfx/vintangle/total?label=binary%20downloads)](https://github.com/pojntfx/vintangle/releases)

## Overview

Vintangle is an app to watch torrents together, similar to how Netflix and Amazon Prime provide a watch party service.

It enables you to ...

- **Stream any torrent**: By utilizing the mpv video player, Vintangle has support for a wide range of video and audio formats.
- **Synchronize playback between remote peers**: Thanks to [weron](https://github.com/pojntfx/weron), Vintangle can be used to host online watch parties by synchronizing playback position, magnet links and other data between peers.
- **Circumvent BitTorrent protocol censorship**: By splitting the core [hTorrent backend](https://github.com/pojntfx/weron) and UI into two separate projects, Vintangle can be used without having to connect a client to the BitTorrent protocol.

## Installation

Vintangle is distributed as a Flatpak. You can install it by running the following:

```shell
# Stable
$ flatpak remote-add vintangle --from "https://pojntfx.github.io/vintangle/flatpak/stable/vintangle.flatpakrepo"
# Unstable
$ flatpak remote-add vintangle --from "https://pojntfx.github.io/vintangle/flatpak/unstable/vintangle.flatpakrepo"
$ flatpak install -y "com.pojtinger.felicitas.vintangle"
```

It will update automatically in the background.

## Usage

🚧 This project is a work-in-progress! Instructions will be added as soon as it is usable. 🚧

## Acknowledgements

- [mpv](https://mpv.io/) provides the media player.
- [diamondburned/gotk4](https://github.com/diamondburned/gotk4) provides the GTK4 bindings for Go.
- [diamondburned/gotk4-adwaita](https://github.com/diamondburned/gotk4-adwaita) provides the `libadwaita` bindings for Go.
- [hTorrent](https://github.com/pojntfx/htorrent) provides the torrent gateway.
- [weron](https://github.com/pojntfx/weron) provides the WebRTC library for playback synchronization.

To all the rest of the authors who worked on the dependencies used: **Thanks a lot!**

## Contributing

To contribute, please use the [GitHub flow](https://guides.github.com/introduction/flow/) and follow our [Code of Conduct](./CODE_OF_CONDUCT.md).

To build and start a development version of Vintangle locally, run the following:

```shell
$ git clone https://github.com/pojntfx/vintangle.git
$ cd vintangle
$ make depend
$ make && sudo make install
$ vintangle
```

Have any questions or need help? Chat with us [on Matrix](https://matrix.to/#/#vintangle:matrix.org?via=matrix.org)!

## License

Vintangle (c) 2022 Felicitas Pojtinger and contributors

SPDX-License-Identifier: AGPL-3.0
