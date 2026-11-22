# Fichiers de test des clés

Clés et certificats **fictifs**, générés pour les tests avec OpenSSL 4.1 et utilisés nulle part ailleurs. Ils servent à vérifier que le paquet lit les fichiers tels qu'OpenSSL les écrit. Mot de passe : `test1234`, sauf `ec.p12` (`pässwörd€`) et `empty.p12` (mot de passe vide).

| Fichier | Contenu |
|---|---|
| `ca.crt` | autorité de test, RSA 2048 |
| `leaf.crt`, `ec.crt` | certificats de signature émis par l'autorité, RSA 2048 et ECDSA P-256 |
| `modern.p12` | clé RSA, `leaf.crt` et `ca.crt` : PBES2, PBKDF2-HMAC-SHA256, AES-256-CBC, MAC SHA-256 (défaut d'OpenSSL 3 et plus) |
| `legacy.p12` | même contenu en `-legacy` : clé en 3DES, certificats en RC2 40 bits, MAC SHA-1 (OpenSSL 1.x, Java, macOS) |
| `ec.p12` | clé ECDSA, `ec.crt` et `ca.crt`, mot de passe non ASCII |
| `empty.p12` | clé RSA et `leaf.crt`, mot de passe vide |
| `leaf-enc.pem` | clé RSA en PKCS#8 chiffré : PBES2, AES-256-CBC, HMAC-SHA256 |
| `ec-enc.pem` | clé ECDSA en PKCS#8 chiffré : PBES2, 3DES, HMAC-SHA1 |

Commandes :

```bash
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -out ca.crt -days 36500 -subj "/CN=ZeRechnung Test CA/O=Example" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign"
openssl req -newkey rsa:2048 -nodes -keyout leaf.key -out leaf.csr -subj "/CN=Exemple Informatique SAS/O=Example/C=FR"
openssl x509 -req -in leaf.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out leaf.crt -days 36500 -extfile ext.cnf
openssl ecparam -name prime256v1 -genkey -noout -out ec.key
openssl req -new -key ec.key -out ec.csr -subj "/CN=Exemple EC/O=Example"
openssl x509 -req -in ec.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out ec.crt -days 36500 -extfile ext.cnf
openssl pkcs12 -export -inkey leaf.key -in leaf.crt -certfile ca.crt -passout pass:test1234 -out modern.p12
openssl pkcs12 -export -legacy -inkey leaf.key -in leaf.crt -certfile ca.crt -passout pass:test1234 -out legacy.p12
openssl pkcs12 -export -inkey ec.key -in ec.crt -certfile ca.crt -passout pass:"pässwörd€" -out ec.p12
openssl pkcs12 -export -inkey leaf.key -in leaf.crt -passout pass: -out empty.p12
openssl pkcs8 -topk8 -v2 aes-256-cbc -v2prf hmacWithSHA256 -in leaf.key -out leaf-enc.pem -passout pass:test1234
openssl pkcs8 -topk8 -v2 des3 -in ec.key -out ec-enc.pem -passout pass:test1234
```

`ext.cnf` contient `basicConstraints=CA:FALSE` et `keyUsage=critical,digitalSignature,nonRepudiation`. Les clés en clair et la clé de l'autorité n'ont pas été gardées.
