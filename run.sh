#!/bin/fish

set LAYOUT_FILE (mktemp)

set layout_content "layout {
    pane split_direction=\"vertical\" {
        pane {
            command \"accountability\"
        }
        pane {
            command \"calcure\"
        }
    }
}
pane_frames false
ui {
    pane_frames {
        hide_session_name true
    }
}
"

echo "$layout_content" > "$LAYOUT_FILE"

zellij --layout "$LAYOUT_FILE"

rm "$LAYOUT_FILE"
