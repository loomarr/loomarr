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
import Ellipsis from "lucide-react-native/icons/ellipsis";
import House from "lucide-react-native/icons/house";
import Inbox from "lucide-react-native/icons/inbox";
import Info from "lucide-react-native/icons/info";
import ListChecks from "lucide-react-native/icons/list-checks";
import LoaderCircle from "lucide-react-native/icons/loader-circle";
import Menu from "lucide-react-native/icons/menu";
import Pause from "lucide-react-native/icons/pause";
import Play from "lucide-react-native/icons/play";
import Plus from "lucide-react-native/icons/plus";
import Search from "lucide-react-native/icons/search";
import Settings from "lucide-react-native/icons/settings";
import SkipBack from "lucide-react-native/icons/skip-back";
import SkipForward from "lucide-react-native/icons/skip-forward";
import Tv from "lucide-react-native/icons/tv";
import Users from "lucide-react-native/icons/users";
import Volume2 from "lucide-react-native/icons/volume-2";
import VolumeX from "lucide-react-native/icons/volume-x";
import X from "lucide-react-native/icons/x";

const icons = {
  // A phone tab header's "this device and who it's signed in as" button (the Device sheet).
  account: CircleUserRound,
  back: ChevronLeft,
  channelDown: ChevronDown,
  channelUp: ChevronUp,
  channels: Tv,
  close: X,
  // The trailing mark of a phone list row that opens or connects to something.
  disclosure: ChevronRight,
  // Web's phone-width bottom bar (#1785, #1659 Shell nav) — the admin/member destinations
  // the sidebar already names with lucide-react glyphs, so the bar reads as the same product.
  filler: Clapperboard,
  guide: BookOpen,
  help: ListChecks,
  home: House,
  info: Info,
  loading: LoaderCircle,
  menu: Menu,
  more: Ellipsis,
  notifications: Bell,
  pause: Pause,
  people: Users,
  play: Play,
  // A phone tab header's plain "add" button (#1840's Requests app bar).
  plus: Plus,
  previous: SkipBack,
  // Requests on every client (#1816's native mock; the web rail and bar draw the same Inbox).
  requests: Inbox,
  search: Search,
  settings: Settings,
  skipForward: SkipForward,
  success: Check,
  time: Clock3,
  volume: Volume2,
  volumeMuted: VolumeX,
  warning: CircleAlert,
} as const;

type IconName = keyof typeof icons;

export type { IconName };
export { icons };
