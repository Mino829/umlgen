package activityfixture;

class Transfers {
    static void hit(String label) { }
    static boolean test() { return true; }

    static void loops(boolean value, int[] items) {
        outer: for (int i = 0; i < 3; i++) {
            while (value) {
                switch (i) {
                    case 0: continue outer;
                    default: break;
                }
                break;
            }
            if (value) break outer;
        }
        do {
            hit("do");
            if (value) continue;
        } while (test());
        for (int item : items) {
            if (value) continue;
            hit("item");
        }
        done: {
            if (value) break done;
            hit("block");
        }
        hit("after");
    }

    static void switches(int n) {
        switch (n) {
            case 0: hit("colon");
            case 1:
                switch (n) {
                    case 2: break;
                    default: hit("inner");
                }
                hit("outer");
                break;
            default: hit("default");
        }
        switch (n) {
            case 0 -> hit("arrow-zero");
            default -> hit("arrow-default");
        }
        hit("after");
    }

    static void literals(boolean value) {
        class Local { void invoke() { hit("local-body"); } }
        Runnable lambda = () -> hit("lambda-body");
        Runnable anonymous = new Runnable() {
            public void run() { hit("anonymous-body"); }
        };
        boolean result = value && test() || test();
        hit("after");
    }

    static int one() { return 1; }
    static int two() { return 2; }
    static int consume(boolean flag, int choice, int assigned) { return choice; }
    static int alternatives(boolean value) {
        int assigned = value ? one() : two();
        return consume(value && test(), value ? one() : two(), assigned);
    }

    static void exceptional(boolean value) {
        if (value) throw new IllegalStateException("explicit");
        hit("normal");
    }

    static void resources() throws Exception {
        try (java.io.StringReader reader = new java.io.StringReader("x")) {
            hit("try-body");
        } catch (Exception error) {
            hit("catch-body");
        } finally {
            hit("finally-body");
        }
        hit("unreachable-after-barrier");
    }

    static void monitor(Object lock) {
        synchronized (lock) { hit("monitor-body"); }
        hit("unreachable-after-barrier");
    }
}
