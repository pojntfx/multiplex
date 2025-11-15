package po

import "embed"

//go:generate sh -c "find .. -name '*.go' -o -name '*.blp' | xgettext --language=C++ --keyword=_ --keyword=L --omit-header -o default.pot --files-from=- && find .. -name '*.desktop.in' | xgettext --language=Desktop --keyword=Name --keyword=Comment --omit-header -j -o default.pot --files-from=- && find .. -name 'metainfo.xml.in' | xgettext --its=/usr/share/gettext/its/metainfo.its --omit-header -j -o default.pot --files-from=- && find .. -name '*.ui' | xgettext --language=Glade --keyword=Name --keyword=Comment --omit-header -j -o default.pot --files-from=-"
//go:generate sh -c "find . -name 'default.po' -print0 | xargs -0 -I {} msgmerge --update --backup=none \"{}\" default.pot"
//go:generate sh -c "find . -type f -name '*.po' -print0 | xargs -0 -I {} sh -c 'msgfmt -o \"{}.mo\" \"{}\"' && find . -type f -name '*.po.mo' -exec sh -c 'mv \"{}\" \"$(echo \"{}\" | sed s/\\.po\\.mo/.mo/)\"' \\;"
//go:embed *
var FS embed.FS
