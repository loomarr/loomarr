import Bell from "lucide-react-native/icons/bell";
import BookOpen from "lucide-react-native/icons/book-open";
import Check from "lucide-react-native/icons/check";
import ChevronDown from "lucide-react-native/icons/chevron-down";
import ChevronLeft from "lucide-react-native/icons/chevron-left";
import ChevronRight from "lucide-react-native/icons/chevron-right";
import ChevronUp from "lucide-react-native/icons/chevron-up";
import CircleAlert from "lucide-react-native/icons/circle-alert";
import CircleUserRound from "lucide-react-native/icons/circle-user-round";
import Clapperboard from "lucide-react-native/icons/clapperboard";
import Clock3 from "lucide-react-native/icons/clock-3";
import Copy from "lucide-react-native/icons/copy";
import Ellipsis from "lucide-react-native/icons/ellipsis";
import House from "lucide-react-native/icons/house";
import Inbox from "lucide-react-native/icons/inbox";
import Info from "lucide-react-native/icons/info";
import Keyboard from "lucide-react-native/icons/keyboard";
import List from "lucide-react-native/icons/list";
import ListChecks from "lucide-react-native/icons/list-checks";
import LoaderCircle from "lucide-react-native/icons/loader-circle";
import Maximize from "lucide-react-native/icons/maximize";
import Menu from "lucide-react-native/icons/menu";
import Minimize from "lucide-react-native/icons/minimize";
import Pause from "lucide-react-native/icons/pause";
import PictureInPicture2 from "lucide-react-native/icons/picture-in-picture-2";
import Play from "lucide-react-native/icons/play";
import Plus from "lucide-react-native/icons/plus";
import RotateCcw from "lucide-react-native/icons/rotate-ccw";
import RotateCw from "lucide-react-native/icons/rotate-cw";
import Search from "lucide-react-native/icons/search";
import Settings from "lucide-react-native/icons/settings";
import SkipBack from "lucide-react-native/icons/skip-back";
import SkipForward from "lucide-react-native/icons/skip-forward";
import Star from "lucide-react-native/icons/star";
import StepBack from "lucide-react-native/icons/step-back";
import Tv from "lucide-react-native/icons/tv";
import Users from "lucide-react-native/icons/users";
import Volume2 from "lucide-react-native/icons/volume-2";
import VolumeX from "lucide-react-native/icons/volume-x";
import X from "lucide-react-native/icons/x";

import { withSeconds } from "./seek-glyph";

const icons = {
  // A phone tab header's "this device and who it's signed in as" button (the Device sheet).
  account: CircleUserRound,
  back: ChevronLeft,
  channelDown: ChevronDown,
  channelUp: ChevronUp,
  channels: Tv,
  close: X,
  // The player's Playback info: Copy stream URL (§9, admins).
  copy: Copy,
  // The trailing mark of a phone list row that opens or connects to something.
  disclosure: ChevronRight,
  // The player's favourite (§9.4, the portrait panel): the tuned channel in the viewer's Favorites.
  favourite: Star,
  // Web's phone-width bottom bar (#1785, #1659 Shell nav) — the admin/member destinations
  // the sidebar already names with lucide-react glyphs, so the bar reads as the same product.
  filler: Clapperboard,
  // The player's window tools (§9.4).
  fullscreen: Maximize,
  fullscreenExit: Minimize,
  guide: BookOpen,
  help: ListChecks,
  home: House,
  info: Info,
  // The player's Keyboard shortcuts sheet (§9.1, the `?` key).
  keyboard: Keyboard,
  // The player's Channels sheet (§9.4): a list of channels, where `channels` is the TV destination.
  list: List,
  loading: LoaderCircle,
  menu: Menu,
  more: Ellipsis,
  notifications: Bell,
  pause: Pause,
  people: Users,
  // The player's window tool (§9.4): the browser's picture-in-picture.
  pictureInPicture: PictureInPicture2,
  play: Play,
  // A phone tab header's plain "add" button (#1840's Requests app bar).
  plus: Plus,
  previous: SkipBack,
  // Requests on every client (#1816's native mock; the web rail and bar draw the same Inbox).
  requests: Inbox,
  search: Search,
  // The player's ±10 s (§9.4): the arrow carries a "10" drawn inside it.
  seekBack: withSeconds(RotateCcw, "10"),
  seekForward: withSeconds(RotateCw, "10"),
  settings: Settings,
  skipForward: SkipForward,
  // The player's Start over (§9.4): back to the programme's start.
  startOver: StepBack,
  success: Check,
  time: Clock3,
  volume: Volume2,
  volumeMuted: VolumeX,
  warning: CircleAlert,
} as const;

type IconName = keyof typeof icons;

export type { IconName };
export { icons };
