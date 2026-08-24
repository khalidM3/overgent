import { cronJobs } from "convex/server";
import { internal } from "./_generated/api";

const crons = cronJobs();

crons.interval("bounded retention sweep", { minutes: 5 }, internal.service.retentionSweep, {});

export default crons;
