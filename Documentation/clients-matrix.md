# Client libraries support matrix for etcd

As etcd features support is really uneven between client libraries, a compatibility matrix can be important.

## v2 clients

The v2 API has a lot of features, we will categorize them in a few categories:

- **HTTPS Auth**: Support for SSL-certificate based authentication
- **Reconnect**: If the client is able to reconnect automatically to another server if one fails.
- **Mod/Lock**: Support for the locking module
- **Mod/Leader**: Support for the leader election module
- **GET,PUT,POST,DEL Features**: Support for all the modifiers when calling the etcd server with said HTTP method.


### Supported features matrix

| Client| [go-etcd](https://github.com/coreos/go-etcd) | [jetcd](https://github.com/diwakergupta/jetcd) | [python-etcd](https://github.com/jplana/python-etcd) | [python-etcd-client](https://github.com/dsoprea/PythonEtcdClient) | [node-etcd](https://github.com/stianeikeland/node-etcd) | [nodejs-etcd](https://github.com/lavagetto/nodejs-etcd) | [etcd-ruby](https://github.com/ranjib/etcd-ruby) | [etcd-api](https://github.com/jdarcy/etcd-api) | [cetcd](https://github.com/dwwoelfel/cetcd) |  [clj-etcd](https://github.com/rthomas/clj-etcd) | [etcetera](https://github.com/drusellers/etcetera)| [Etcd.jl](https://github.com/forio/Etcd.jl) | [p5-etcd](https://metacpan.org/release/Etcd) | [justinsb/jetcd](https://github.com/justinsb/jetcd) | [txetcd](https://github.com/russellhaering/txetcd)
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **HTTPS Auth**    | Y | Y | Y | Y | Y | Y | - | - | - | - | - | - | - | - | - |
| **Reconnect**     | Y | - | Y | Y | - | - | - | Y | - | - | - | - | - | - | - |
| **Mod/Lock**      | - | - | Y | Y | - | - | - | - | - | - | - | Y | - | - | - |
| **Mod/Leader**    | - | - | - | Y | - | - | - | - | - | - | - | Y | - | - | - |
| **GET Features**  | F | B | F | F | F | F | F | B | F | G | F | F | F | B | G |
| **PUT Features**  | F | B | F | F | F | F | F | G | F | G | F | F | F | B | G |
| **POST Features** | F | - | F | F | - | F | F | - | - | - | F | F | F | - | F |
| **DEL Features**  | F | B | F | F | F | F | F | B | G | B | F | F | F | B | G |

**Legend**

**F**: Full support **G**: Good support **B**: Basic support
**Y**: Feature supported  **-**: Feature not supported
