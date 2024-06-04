import Adw from "gi://Adw?version=1";
import GObject from "gi://GObject";
import GLib from "gi://GLib";
import Gio from "gi://Gio";
import Gtk from "gi://Gtk?version=4.0";

const application = new Adw.Application({
  application_id: "com.pojtinger.felicitas.multiplex.mockups.playback",
});

const dirname = GLib.path_get_dirname(
  GLib.Uri.parse(import.meta.url, GLib.UriFlags.NONE).get_path()
);

const Window = GObject.registerClass(
  {
    GTypeName: "MultiplexWindow",
    Template: GLib.filename_to_uri(
      GLib.build_filenamev([dirname, "window.ui"]),
      null
    ),
    InternalChildren: ["video_player", "video_metadata", "video_controls"],
  },
  class MultiplexWindow extends Adw.Window {
    constructor(...args) {
      super(...args);

      let hideTimeout;
      const resetTimeout = () => {
        if (hideTimeout) {
          GLib.source_remove(hideTimeout);
        }

        hideTimeout = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, 3, () => {
          this._video_metadata.reveal_child = false;
          this._video_controls.reveal_child = false;

          hideTimeout = undefined;

          return GLib.SOURCE_REMOVE;
        });
      };
      resetTimeout();

      const controller = new Gtk.EventControllerMotion();
      controller.connect("motion", () => {
        this._video_metadata.reveal_child = true;
        this._video_controls.reveal_child = true;

        resetTimeout();
      });
      this.add_controller(controller);
    }

    get videoPlayer() {
      return this._video_player;
    }
  }
);

application.connect("activate", () => {
  const window = new Window({
    application,
  });

  window.videoPlayer.file = Gio.File.new_for_uri(
    GLib.filename_to_uri(GLib.build_filenamev([dirname, "video.webp"]), null)
  );

  const css = new Gtk.CssProvider();
  css.load_from_path(GLib.build_filenamev([dirname, "main.css"]));

  Gtk.StyleContext.add_provider_for_display(
    window.get_display(),
    css,
    Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION
  );

  window.show();
});

application.run([]);
