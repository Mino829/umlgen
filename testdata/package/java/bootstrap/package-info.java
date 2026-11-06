// Type-zero source: imports are still package dependencies.
package sample.bootstrap;

import sample.service.Service;
// No classpath is loaded: this import remains unresolved, not an external node.
import missing.remote.Client;
