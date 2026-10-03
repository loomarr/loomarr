import Bell from "lucide-react-native/icons/bell";
import BookOpen from "lucide-react-native/icons/book-open";
import Check from "lucide-react-native/icons/check";
import ChevronDown from "lucide-react-native/icons/chevron-down";
import ChevronLeft from "lucide-react-native/icons/chevron-left";
import ChevronUp from "lucide-react-native/icons/chevron-up";
import CircleAlert from "lucide-react-native/icons/circle-alert";
import Clapperboard from "lucide-react-native/icons/clapperboard";
import Clock3 from "lucide-react-native/icons/clock-3";
import Ellipsis from "lucide-react-native/icons/ellipsis";
import House from "lucide-react-native/icons/house";
import Info from "lucide-react-native/icons/info";
import LayoutGrid from "lucide-react-native/icons/layout-grid";
import ListChecks from "lucide-react-native/icons/list-checks";
import LoaderCircle from "lucide-react-native/icons/loader-circle";
import Menu from "lucide-react-native/icons/menu";
import Pause from "lucide-react-native/icons/pause";
import Play from "lucide-react-native/icons/play";
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
  back: ChevronLeft,
  channelDown: ChevronDown,
  channelUp: ChevronUp,
  channels: Tv,
  close: X,
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
  previous: SkipBack,
  requests: LayoutGrid,
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
