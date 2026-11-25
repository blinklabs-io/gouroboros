{-# LANGUAGE TypeApplications #-}

-- Copyright 2026 Blink Labs Software
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--     http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

-- Run in the cardano-ledger Cabal environment at
-- 309978c46b4ef937d5c0eefda8be6164ea09573e with GHC 9.6.7:
-- cabal exec -- runghc /path/to/nonce_reference.hs
-- This calls the reference nonce functions, not a copy of their formulas.
import Cardano.Crypto.Hash (hashFromBytes)
import Cardano.Crypto.Util (bytesToNatural)
import Cardano.Crypto.VRF
import Cardano.Ledger.BaseTypes
import Cardano.Protocol.Crypto (StandardCrypto, VRF)
import Cardano.Protocol.Praos.VRF (vrfNonceValue)
import qualified Data.ByteString as BS
import Data.Maybe (fromJust)
import Data.Proxy (Proxy (Proxy))

main :: IO ()
main = do
  let output = mkTestOutputVRF @(VRF StandardCrypto)
        (bytesToNatural (BS.replicate 64 2))
      -- Nonce derivation only reads the output. This proof is not verified.
      proof = fromJust $ rawDeserialiseCertVRF @(VRF StandardCrypto)
        (BS.replicate 80 0)
      tp = mkNonceFromOutputVRF output
      pr = vrfNonceValue (Proxy @StandardCrypto) (CertifiedVRF output proof)
      previous = Nonce $ fromJust $ hashFromBytes (BS.replicate 32 1)
      zero = Nonce $ fromJust $ hashFromBytes (BS.replicate 32 0)
  print (getOutputVRFBytes output == BS.replicate 64 2)
  mapM_ (\(label, value) -> putStrLn (label <> ": " <> show value))
    [ ("TPraos contribution", tp)
    , ("Praos contribution", pr)
    , ("TPraos evolving", previous ⭒ tp)
    , ("Praos evolving", previous ⭒ pr)
    , ("TPraos zero hash", zero ⭒ tp)
    , ("Praos zero hash", zero ⭒ pr)
    , ("Neutral TPraos", NeutralNonce ⭒ tp)
    , ("Neutral Praos", NeutralNonce ⭒ pr)
    , ("Chained TPraos to Praos", (previous ⭒ tp) ⭒ pr)
    ]
