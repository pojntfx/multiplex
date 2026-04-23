package po

import "embed"

//go:generate sh -c "find .. -mindepth 1 \\( -name '.*' -o -name builddir \\) -prune -o \\( -name '*.go' -o -name '*.blp' \\) -print | xgettext --language=C++ --keyword=_ --keyword=L --add-comments=TRANSLATORS: --omit-header -o multiplex.pot --files-from=- && find .. -mindepth 1 \\( -name '.*' -o -name builddir \\) -prune -o -name '*.desktop.in' -print | xgettext --language=Desktop --keyword=Name --keyword=Comment --add-comments=TRANSLATORS: --omit-header -j -o multiplex.pot --files-from=- && find .. -mindepth 1 \\( -name '.*' -o -name builddir \\) -prune -o -name 'metainfo.xml.in' -print | xgettext --its=/usr/share/gettext/its/metainfo.its --omit-header -j -o multiplex.pot --files-from=- && find .. -mindepth 1 \\( -name '.*' -o -name builddir \\) -prune -o -name '*.ui' -print | xgettext --language=Glade --keyword=Name --keyword=Comment --omit-header -j -o multiplex.pot --files-from=- && find .. -mindepth 1 \\( -name '.*' -o -name builddir \\) -prune -o -name '*.gschema.xml' -print | xgettext --its=/usr/share/gettext/its/gschema.its --omit-header -j -o multiplex.pot --files-from=-"
//go:generate sh -c "find . -mindepth 1 -name '.*' -prune -o -name '*.po' -print0 | xargs -0 -I {} msgmerge --update --backup=none \"{}\" multiplex.pot"
//go:generate sh -c "find . -mindepth 1 -name '.*' -prune -o -type f -name '*.po' -print0 | xargs -0 -I {} sh -c 'mkdir -p $(basename {} .po)/LC_MESSAGES && msgfmt -o $(basename {} .po)/LC_MESSAGES/multiplex.mo {}'"
//go:generate sh -c "find . -mindepth 1 -name '.*' -prune -o -name \"*.po\" -exec basename {} .po \\; > LINGUAS"
//go:embed *
var FS embed.FS
