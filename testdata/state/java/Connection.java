package fixture;

class Connection {
    enum Phase { NEW, OPEN, CLOSED, UNUSED }
    Phase state = Phase.NEW;

    Connection() { state = Phase.NEW; }

    void connect(boolean ready) {
        if (ready) state = Phase.OPEN;
        else state = unknown();
    }

    void close(int reason) {
        switch (reason) {
            case 1: state = Phase.CLOSED; break;
            default: state = Phase.NEW;
        }
    }

    void other(Connection other) { other.state = Phase.CLOSED; }
    Runnable later() { return () -> state = Phase.OPEN; }
    void guarded() { synchronized (this) { state = Phase.CLOSED; } }
    void afterBarrier() { try { state = Phase.OPEN; } finally {} state = Phase.CLOSED; }
    Phase unknown() { return null; }
}
