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
import Download from "lucide-react-native/icons/download";
import Ellipsis from "lucide-react-native/icons/ellipsis";
import ExternalLink from "lucide-react-native/icons/external-link";
import Eye from "lucide-react-native/icons/eye";
import EyeOff from "lucide-react-native/icons/eye-off";
import House from "lucide-react-native/icons/house";
import Inbox from "lucide-react-native/icons/inbox";
import Info from "lucide-react-native/icons/info";
import Library from "lucide-react-native/icons/library";
import List from "lucide-react-native/icons/list";
import ListChecks from "lucide-react-native/icons/list-checks";
import ListFilter from "lucide-react-native/icons/list-filter";
import LoaderCircle from "lucide-react-native/icons/loader-circle";
import Maximize from "lucide-react-native/icons/maximize";
import Menu from "lucide-react-native/icons/menu";
import Minimize from "lucide-react-native/icons/minimize";
import PanelRightClose from "lucide-react-native/icons/panel-right-close";
import PanelRightOpen from "lucide-react-native/icons/panel-right-open";
import Pause from "lucide-react-native/icons/pause";
import Pencil from "lucide-react-native/icons/pencil";
import Play from "lucide-react-native/icons/play";
import Plus from "lucide-react-native/icons/plus";
import Radio from "lucide-react-native/icons/radio";
import RadioTower from "lucide-react-native/icons/radio-tower";
import RefreshCw from "lucide-react-native/icons/refresh-cw";
import RotateCcw from "lucide-react-native/icons/rotate-ccw";
import RotateCw from "lucide-react-native/icons/rotate-cw";
import Search from "lucide-react-native/icons/search";
import Settings from "lucide-react-native/icons/settings";
import SkipBack from "lucide-react-native/icons/skip-back";
import SkipForward from "lucide-react-native/icons/skip-forward";
import Sparkles from "lucide-react-native/icons/sparkles";
import Square from "lucide-react-native/icons/square";
import StepBack from "lucide-react-native/icons/step-back";
import Trash from "lucide-react-native/icons/trash";
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
  // Diagnostics' verbose log capture, which records like a broadcast does.
  capture: Radio,
  // A password Field's show and hide button.
  conceal: EyeOff,
  copy: Copy,
  delete: Trash,
  // The trailing mark of a phone list row that opens or connects to something.
  disclosure: ChevronRight,
  download: Download,
  edit: Pencil,
  // A link that leaves Loomarr (an Emby page, a provider's site).
  external: ExternalLink,
  // Web's phone-width bottom bar (#1785, #1659 Shell nav) — the admin/member destinations
  // the sidebar already names with lucide-react glyphs, so the bar reads as the same product.
  filler: Clapperboard,
  // A toolbar's "More filters" disclosure.
  filter: ListFilter,
  // The player's window tools (§9.4).
  fullscreen: Maximize,
  fullscreenExit: Minimize,
  guide: BookOpen,
  help: ListChecks,
  home: House,
  info: Info,
  // Web's Library destination (#2151), drawn as the sidebar draws it.
  library: Library,
  // The player's Channels sheet (§9.4): a list of channels, where `channels` is the TV destination.
  list: List,
  loading: LoaderCircle,
  menu: Menu,
  more: Ellipsis,
  notifications: Bell,
  // A list page's pager: toward newer rows and toward older ones.
  pageNewer: ChevronLeft,
  pageOlder: ChevronRight,
  // A split view's details pane, hidden and shown.
  paneClose: PanelRightClose,
  paneOpen: PanelRightOpen,
  pause: Pause,
  people: Users,
  play: Play,
  // A phone tab header's plain "add" button (#1840's Requests app bar).
  plus: Plus,
  previous: SkipBack,
  // A playout process's own output, opened from a log line that names it.
  process: RadioTower,
  refresh: RefreshCw,
  // Requests on every client (#1816's native mock; the web rail and bar draw the same Inbox).
  requests: Inbox,
  reveal: Eye,
  search: Search,
  // The player's ±10 s (§9.4): the arrow carries a "10" drawn inside it.
  seekBack: withSeconds(RotateCcw, "10"),
  seekForward: withSeconds(RotateCw, "10"),
  settings: Settings,
  skipForward: SkipForward,
  // The player's Start over (§9.4): back to the programme's start.
  startOver: StepBack,
  stop: Square,
  success: Check,
  // Asking the model for something: the AI action, never a plain primary (§2.1).
  suggest: Sparkles,
  time: Clock3,
  volume: Volume2,
  volumeMuted: VolumeX,
  warning: CircleAlert,
} as const;

type IconName = keyof typeof icons;

export type { IconName };
export { icons };
