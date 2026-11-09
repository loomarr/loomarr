import Activity from "lucide-react-native/icons/activity";
import Archive from "lucide-react-native/icons/archive";
import ArrowLeft from "lucide-react-native/icons/arrow-left";
import ArrowRight from "lucide-react-native/icons/arrow-right";
import ArrowUpRight from "lucide-react-native/icons/arrow-up-right";
import Bell from "lucide-react-native/icons/bell";
import BookOpen from "lucide-react-native/icons/book-open";
import CalendarClock from "lucide-react-native/icons/calendar-clock";
import Check from "lucide-react-native/icons/check";
import ChevronDown from "lucide-react-native/icons/chevron-down";
import ChevronLeft from "lucide-react-native/icons/chevron-left";
import ChevronRight from "lucide-react-native/icons/chevron-right";
import ChevronUp from "lucide-react-native/icons/chevron-up";
import Circle from "lucide-react-native/icons/circle";
import CircleAlert from "lucide-react-native/icons/circle-alert";
import CircleCheck from "lucide-react-native/icons/circle-check";
import CircleUserRound from "lucide-react-native/icons/circle-user-round";
import CircleX from "lucide-react-native/icons/circle-x";
import Clapperboard from "lucide-react-native/icons/clapperboard";
import Clock3 from "lucide-react-native/icons/clock-3";
import Copy from "lucide-react-native/icons/copy";
import Database from "lucide-react-native/icons/database";
import Download from "lucide-react-native/icons/download";
import Ellipsis from "lucide-react-native/icons/ellipsis";
import ExternalLink from "lucide-react-native/icons/external-link";
import Eye from "lucide-react-native/icons/eye";
import EyeOff from "lucide-react-native/icons/eye-off";
import FolderOpen from "lucide-react-native/icons/folder-open";
import GripVertical from "lucide-react-native/icons/grip-vertical";
import HeartPulse from "lucide-react-native/icons/heart-pulse";
import House from "lucide-react-native/icons/house";
import ImageOff from "lucide-react-native/icons/image-off";
import Inbox from "lucide-react-native/icons/inbox";
import Info from "lucide-react-native/icons/info";
import KeyRound from "lucide-react-native/icons/key-round";
import LayoutGrid from "lucide-react-native/icons/layout-grid";
import Library from "lucide-react-native/icons/library";
import List from "lucide-react-native/icons/list";
import ListChecks from "lucide-react-native/icons/list-checks";
import ListFilter from "lucide-react-native/icons/list-filter";
import LoaderCircle from "lucide-react-native/icons/loader-circle";
import Lock from "lucide-react-native/icons/lock";
import LockOpen from "lucide-react-native/icons/lock-open";
import LogOut from "lucide-react-native/icons/log-out";
import Mail from "lucide-react-native/icons/mail";
import Maximize from "lucide-react-native/icons/maximize";
import Menu from "lucide-react-native/icons/menu";
import Minimize from "lucide-react-native/icons/minimize";
import Minus from "lucide-react-native/icons/minus";
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
import Scissors from "lucide-react-native/icons/scissors";
import Search from "lucide-react-native/icons/search";
import Server from "lucide-react-native/icons/server";
import Settings from "lucide-react-native/icons/settings";
import Settings2 from "lucide-react-native/icons/settings-2";
import ShieldAlert from "lucide-react-native/icons/shield-alert";
import ShieldCheck from "lucide-react-native/icons/shield-check";
import SkipBack from "lucide-react-native/icons/skip-back";
import SkipForward from "lucide-react-native/icons/skip-forward";
import Sparkles from "lucide-react-native/icons/sparkles";
import Square from "lucide-react-native/icons/square";
import StepBack from "lucide-react-native/icons/step-back";
import Trash from "lucide-react-native/icons/trash";
import TriangleAlert from "lucide-react-native/icons/triangle-alert";
import Tv from "lucide-react-native/icons/tv";
import UserRound from "lucide-react-native/icons/user-round";
import Users from "lucide-react-native/icons/users";
import Volume2 from "lucide-react-native/icons/volume-2";
import VolumeX from "lucide-react-native/icons/volume-x";
import X from "lucide-react-native/icons/x";

import { withSeconds } from "./seek-glyph";

const icons = {
  // A phone tab header's "this device and who it's signed in as" button (the Device sheet).
  account: CircleUserRound,
  // Diagnostics' live activity pulse.
  activity: Activity,
  // A caution the reader should act on or know about, drawn as a triangle: `warning` is the round
  // mark of a status, this is the banner's.
  alert: TriangleAlert,
  // A stored bundle of files (a filler source kept as an archive).
  archive: Archive,
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
  // The database Loomarr stores its state in.
  database: Database,
  delete: Trash,
  // The trailing mark of a phone list row that opens or connects to something.
  disclosure: ChevronRight,
  download: Download,
  // The grip of a row or a divider that is dragged to reorder or resize.
  dragHandle: GripVertical,
  // The guide's hour stepper: the window an hour back in time, and an hour on.
  earlier: ChevronLeft,
  edit: Pencil,
  // A link that leaves Loomarr (an Emby page, a provider's site).
  external: ExternalLink,
  // A check that did not pass, beside `passed` and `pending`.
  failed: CircleX,
  // Web's phone-width bottom bar (#1785, #1659 Shell nav) — the admin/member destinations
  // the sidebar already names with lucide-react glyphs, so the bar reads as the same product.
  filler: Clapperboard,
  // A toolbar's "More filters" disclosure.
  filter: ListFilter,
  // The player's window tools (§9.4).
  fullscreen: Maximize,
  fullscreenExit: Minimize,
  // A folder on disk (a local filler source).
  folder: FolderOpen,
  // A back link that names where it goes, drawn as an arrow: `back` is the chevron of a bar.
  goBack: ArrowLeft,
  goForward: ArrowRight,
  // A link that carries on elsewhere in the app ("Fix" in a checklist).
  goTo: ArrowUpRight,
  // A catalog's card wall, the alternative to its `list` rows.
  grid: LayoutGrid,
  guide: BookOpen,
  // Diagnostics' health view.
  health: HeartPulse,
  help: ListChecks,
  home: House,
  info: Info,
  // A credential, a sign-in code or a password.
  key: KeyRound,
  later: ChevronRight,
  // Web's Library destination (#2151), drawn as the sidebar draws it.
  library: Library,
  // The player's Channels sheet (§9.4): a list of channels, where `channels` is the TV destination.
  list: List,
  loading: LoaderCircle,
  // A setting or a thing the installation fixes and the reader cannot change here.
  locked: Lock,
  mail: Mail,
  menu: Menu,
  more: Ellipsis,
  // A picture that is missing or failed to load.
  noImage: ImageOff,
  notifications: Bell,
  // A list page's pager: toward newer rows and toward older ones.
  pageNewer: ChevronLeft,
  pageOlder: ChevronRight,
  // A split view's details pane, hidden and shown.
  paneClose: PanelRightClose,
  paneOpen: PanelRightOpen,
  pause: Pause,
  // A check that has passed, beside `failed` and `pending`.
  passed: CircleCheck,
  // One person; `people` is the household.
  person: UserRound,
  pending: Circle,
  people: Users,
  // A table row's disclosure: closed points at the row's details, open points down into them.
  rowCollapse: ChevronDown,
  rowExpand: ChevronRight,
  play: Play,
  // A phone tab header's plain "add" button (#1840's Requests app bar).
  plus: Plus,
  previous: SkipBack,
  // A playout process's own output, opened from a log line that names it.
  process: RadioTower,
  refresh: RefreshCw,
  // Takes an item out of a set without deleting it (a rule in a diff, "removed").
  remove: Minus,
  // Requests on every client (#1816's native mock; the web rail and bar draw the same Inbox).
  requests: Inbox,
  reveal: Eye,
  // A change that needs the reader's say before it can go ahead.
  risk: ShieldAlert,
  // A date and time, as an invitation's expiry.
  schedule: CalendarClock,
  search: Search,
  // The mark on the chosen option of a list.
  selected: Check,
  // A machine or service Loomarr talks to.
  server: Server,
  // The player's ±10 s (§9.4): the arrow carries a "10" drawn inside it.
  seekBack: withSeconds(RotateCcw, "10"),
  seekForward: withSeconds(RotateCw, "10"),
  settings: Settings,
  // A control that opens a form's finer options.
  settingsAdjust: Settings2,
  signOut: LogOut,
  skipForward: SkipForward,
  // Cutting one recording into its clips (filler's Split into clips).
  split: Scissors,
  // The player's Start over (§9.4): back to the programme's start.
  startOver: StepBack,
  stop: Square,
  success: Check,
  // Asking the model for something: the AI action, never a plain primary (§2.1).
  suggest: Sparkles,
  time: Clock3,
  // The opposite of `locked`: a setting the reader may change.
  unlocked: LockOpen,
  // Checked and safe: a verified, private report.
  verified: ShieldCheck,
  volume: Volume2,
  volumeMuted: VolumeX,
  warning: CircleAlert,
} as const;

type IconName = keyof typeof icons;

export type { IconName };
export { icons };
