import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { DateScope } from "@loomarr/api/models/dateScope";
import type { FillerSelection } from "@loomarr/api/models/fillerSelection";

// Date-window states shared by the web and future mobile component workshops. These are
// deliberately fixed domain data so visual baselines do not depend on a live channel clock.
const disjointFillerCriteria: { selection: FillerSelection; programmingDates: DateScope } = {
  selection: {
    eraWindows: [
      { from: 1990, to: 1999 },
      { from: 2005, to: 2009 },
    ],
  },
  programmingDates: {
    movieRelease: [{ from: 1990, to: 1999 }],
    seriesAiring: [{ from: 2005, to: 2009 }],
  },
};

const disjointProgrammingDatesPolicy: ChannelPolicy = {
  scope: {
    dates: {
      movieRelease: [
        { from: 1990, to: 1999 },
        { from: 2005, to: 2009 },
      ],
      seriesAiring: [{ from: 2010, to: 2014 }],
    },
  },
};

export { disjointFillerCriteria, disjointProgrammingDatesPolicy };
