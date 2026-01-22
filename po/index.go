package po

import "embed"

//go:generate sh -c "find .. -name '*.go' -o -name '*.blp' | xgettext --language=C++ --keyword=_ --keyword=L --add-comments=TRANSLATORS: --omit-header -o multiplex.pot --files-from=- && find .. -name '*.desktop.in' | xgettext --language=Desktop --keyword=Name --keyword=Comment --add-comments=TRANSLATORS: --omit-header -j -o multiplex.pot --files-from=- && find .. -name 'metainfo.xml.in' | xgettext --its=/usr/share/gettext/its/metainfo.its --omit-header -j -o multiplex.pot --files-from=- && find .. -name '*.ui' | xgettext --language=Glade --keyword=Name --keyword=Comment --omit-header -j -o multiplex.pot --files-from=- && find .. -name '*.gschema.xml' | xgettext --its=/usr/share/gettext/its/gschema.its --omit-header -j -o multiplex.pot --files-from=-"
//go:generate sh -c "find . -name '*.po' -print0 | xargs -0 -I {} msgmerge --update --backup=none \"{}\" multiplex.pot"
//go:generate sh -c "find . -type f -name '*.po' -print0 | xargs -0 -I {} sh -c 'mkdir -p $(basename {} .po)/LC_MESSAGES && msgfmt -o $(basename {} .po)/LC_MESSAGES/multiplex.mo {}'"
//go:generate sh -c "find . -name \"*.po\" -exec basename {} .po \\; > LINGUAS"
//go:embed *
var FS embed.FS
