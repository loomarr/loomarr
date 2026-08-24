import BookOpen from "lucide-react-native/icons/book-open";
import Check from "lucide-react-native/icons/check";
import ChevronLeft from "lucide-react-native/icons/chevron-left";
import CircleAlert from "lucide-react-native/icons/circle-alert";
import Clock3 from "lucide-react-native/icons/clock-3";
import House from "lucide-react-native/icons/house";
import Info from "lucide-react-native/icons/info";
import LoaderCircle from "lucide-react-native/icons/loader-circle";
import Menu from "lucide-react-native/icons/menu";
import Pause from "lucide-react-native/icons/pause";
import Play from "lucide-react-native/icons/play";
import Search from "lucide-react-native/icons/search";
import Settings from "lucide-react-native/icons/settings";
import SkipBack from "lucide-react-native/icons/skip-back";
import SkipForward from "lucide-react-native/icons/skip-forward";
import Tv from "lucide-react-native/icons/tv";
import Volume2 from "lucide-react-native/icons/volume-2";
import VolumeX from "lucide-react-native/icons/volume-x";
import X from "lucide-react-native/icons/x";

const icons = {
  back: ChevronLeft,
  channels: Tv,
  close: X,
  guide: BookOpen,
  home: House,
  info: Info,
  loading: LoaderCircle,
  menu: Menu,
  pause: Pause,
  play: Play,
  previous: SkipBack,
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
